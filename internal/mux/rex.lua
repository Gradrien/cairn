-- cairn's bridge to Rex: one `rex do` per operation, rex.args.op says which.
-- Every result is a table; a failure is { error = "…" } rather than a raised
-- error, so the caller gets the message whole.
local T = "com.superlogical.terminal"
local args = rex.args
local op = args.op

local function sessions()
  local list, err = rex.call("session.list", {})
  if err ~= nil then error(err, 0) end
  return list.sessions or {}
end

-- blocks calls fn(session, window, block) for every terminal block, first
-- window first; fn returning true stops the walk.
local function blocks(s, fn)
  -- A script only sees the sessions it is attached to; attaching returns the view.
  local att = rex.call("session.attach", { session_id = s.session_id })
  local view = att and att.view
  if view == nil then view = rex.call("session.view", { session_id = s.session_id }) end
  if view == nil then return false end
  for _, w in ipairs(view.windows or {}) do
    for _, l in ipairs(w.layers or {}) do
      for _, b in ipairs(l.blocks or {}) do
        if b.creator_name == T and fn(s, w, b, view) then return true end
      end
    end
  end
  return false
end

-- where finds the session holding a block.
local function where(block_id)
  for _, s in ipairs(sessions()) do
    local found
    blocks(s, function(s, w, b)
      if b.block_id == block_id then found = { session_id = s.session_id, window_id = w.window_id } return true end
    end)
    if found then return found end
  end
  return nil
end

local function call(sid, bid, method, a)
  a = a or {}
  a.session_id, a.block_id = sid, bid
  return rex.block.call(T, method, a)
end

local function describe(s, w, b)
  local out = { session_id = s.session_id, window_id = w.window_id, block_id = b.block_id, label = b.label or "" }
  local p = call(s.session_id, b.block_id, "process")
  if p ~= nil then
    if p.foreground ~= nil then
      out.argv0, out.name, out.cwd = p.foreground.argv0 or "", p.foreground.name or "", p.foreground.cwd or ""
    end
    if p.ancestors ~= nil and p.ancestors[1] ~= nil then out.shell_cwd = p.ancestors[1].cwd or "" end
  end
  local st = call(s.session_id, b.block_id, "program_status")
  out.records = {}
  if st ~= nil and type(st.records) == "table" then
    for _, r in ipairs(st.records) do
      table.insert(out.records, { id = r.id or "", state = r.state or "", app = r.effective_app or r.app or "" })
    end
  end
  local t = call(s.session_id, b.block_id, "title")
  if t ~= nil then out.title = t.title or "" end
  return out
end

local function target()
  local w = where(args.block)
  if w == nil then error("block " .. tostring(args.block) .. " not found", 0) end
  rex.call("session.attach", { session_id = w.session_id })
  return w.session_id, args.block, w.window_id
end

local ops = {}

function ops.snapshot()
  local out = { sessions = {}, blocks = {} }
  for _, s in ipairs(sessions()) do
    table.insert(out.sessions, { session_id = s.session_id, label = s.label or "" })
    blocks(s, function(s, w, b) table.insert(out.blocks, describe(s, w, b)) end)
  end
  return out
end

function ops.block()
  for _, s in ipairs(sessions()) do
    local found
    blocks(s, function(s, w, b)
      if b.block_id == args.block then found = describe(s, w, b) return true end
    end)
    if found then return found end
  end
  return { missing = true }
end

function ops.first_block()
  for _, s in ipairs(sessions()) do
    if s.session_id == args.session then
      local found
      blocks(s, function(s, w, b) found = b.block_id return true end)
      if found then return { block_id = found } end
      return { error = "session " .. args.session .. " has no terminal" }
    end
  end
  return { error = "session " .. tostring(args.session) .. " not found" }
end

-- prompt pastes the text, so newlines stay inside it, then presses Return.
function ops.prompt()
  local sid, bid = target()
  local _, err = rex.call("block.event", { session_id = sid, block_id = bid, event = { type = "paste", payload = { text = args.text } } })
  if err ~= nil then return { error = err } end
  rex.sleep(0.25)
  local _, err = call(sid, bid, "write", { data = "\r" })
  if err ~= nil then return { error = err } end
  return {}
end

function ops.write()
  local sid, bid = target()
  local _, err = call(sid, bid, "write", { data = args.data })
  if err ~= nil then return { error = err } end
  return {}
end

function ops.label()
  local sid, bid = target()
  local _, err = rex.session.set_block_label{ session_id = sid, block_id = bid, label = args.label }
  if err ~= nil then return { error = err } end
  return {}
end

function ops.session_label()
  rex.call("session.attach", { session_id = args.session })
  local _, err = rex.call("session.set_label", { session_id = args.session, label = args.label })
  if err ~= nil then return { error = err } end
  return {}
end

-- focus brings a block to the front of its session and says where it is; the
-- app is switched to that session by the caller (a client action).
function ops.focus()
  local sid, wid = args.session, nil
  if args.block ~= nil and args.block ~= "" then
    local bid
    sid, bid, wid = target()
    local _, err = rex.call("session.focus_block", { session_id = sid, block_id = bid })
    if err ~= nil then return { error = err } end
  end
  return { session_id = sid, window_id = wid }
end

local fn = ops[op]
if fn == nil then error("unknown op " .. tostring(op), 0) end
return fn()
