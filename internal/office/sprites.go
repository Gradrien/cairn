package office

import "hash/fnv"

// The office keeps its own colours, the same in a light or a dark terminal:
// a warm room, framed by cairn's interface around it.
const (
	wall      Color = "#d8cab2"
	wallLine  Color = "#cbbb9f"
	baseboard Color = "#8a6d4b"
	floor     Color = "#b38b62"
	floorLine Color = "#a37c55"
	ink       Color = "#1f2937"
	paper     Color = "#f7f5ef"
	bubbleBg  Color = "#fbfaf6"
	warnBg    Color = "#ffb86c"
	dimInk    Color = "#6b7280"
	ledOn     Color = "#50fa7b"
	ledOff    Color = "#3a3d48"
	ledRed    Color = "#ff5555"
	ledAmber  Color = "#f1fa8c"
)

// A person, 6×8, facing you. Frame b is a step: legs apart, a pixel higher.
var (
	personA = Sprite{
		".hhhh.",
		"hhhhhh",
		"hesseh",
		".ssss.",
		"sccccs",
		".cccc.",
		".p..p.",
		".p..p.",
	}
	personB = Sprite{
		".hhhh.",
		"hhhhhh",
		"hesseh",
		".ssss.",
		"sccccs",
		".cccc.",
		".p..p.",
		"p....p",
	}
	// Arms up: asking for a decision.
	personAsk = Sprite{
		".hhhh.",
		"hhhhhh",
		"hesseh",
		"s.ss.s",
		"cccccc",
		".cccc.",
		".p..p.",
		".p..p.",
	}
)

// A desk seen from the front: the back of a screen on the left, the table.
var desk = Sprite{
	".mmmmm........",
	".mmmmm........",
	".mmmmm........",
	"...m..........",
	"tttttttttttttt",
	"TTTTTTTTTTTTTT",
	"dPPPPPPPPPPPPd",
	"d............d",
}

const deskW, deskH = 14, 8

var deskPal = map[byte]Color{'m': "#2d2f3a", 't': "#9a7a58", 'T': "#7a5c40", 'P': "#6b4f36", 'd': "#5a4230"}

var window = Sprite{
	"fffffffffffff",
	"fsssssfsssssf",
	"fsssssfsssssf",
	"fsscssfssssff",
	"fffffffffffff",
	"fsssssfsssssf",
	"fssssgfggsssf",
	"fsggggfggggsf",
	"fffffffffffff",
}

var windowPal = map[byte]Color{'f': "#f4efe6", 's': "#9fd3f0", 'c': "#ffffff", 'g': "#7fb77e"}

// The back of an empty chair, above the table.
var chair = Sprite{
	".aaa.",
	".aaa.",
	".aaa.",
}

var chairPal = map[byte]Color{'a': "#4b5563"}

var shelf = Sprite{
	"ffffffffffffffff",
	"frrgbbyorrgbyogf",
	"frrgbbyorrgbyogf",
	"ffffffffffffffff",
	"fbyogrrbgyobrrgf",
	"fbyogrrbgyobrrgf",
	"ffffffffffffffff",
}

var shelfPal = map[byte]Color{'f': "#7a5a3a", 'r': "#c0504d", 'g': "#4f9a6a", 'b': "#4a78b5", 'y': "#d9b44a", 'o': "#d9813a"}

// The test machine; its lights are drawn on top.
var machine = Sprite{
	"kkkkkkkk",
	"k......k",
	"kkkkkkkk",
	"kSSSSSSk",
	"kkkkkkkk",
	"kSSSSSSk",
	"kkkkkkkk",
	"kSSSSSSk",
	"kkkkkkkk",
	"kk....kk",
}

var machinePal = map[byte]Color{'k': "#4a4e5a", 'S': "#2b2e37", '.': "#2b2e37"}

var plant = Sprite{
	".g.g.",
	"ggggg",
	".ggg.",
	"..g..",
	".ooo.",
	".ooo.",
}

var plantPal = map[byte]Color{'g': "#4f9a6a", 'o': "#a0583a"}

var door = Sprite{
	"ffffffffff",
	"fpppppppff",
	"fpqqqqqpff",
	"fpqqqqqpff",
	"fpppppppff",
	"fpqqqqqpff",
	"fpqqqqqpff",
	"fpqqqqqpff",
	"fppppppkff",
	"fpqqqqqpff",
	"fpqqqqqpff",
	"fpqqqqqpff",
	"fpqqqqqpff",
	"fpppppppff",
	"fpppppppff",
	"fpppppppff",
}

var doorPal = map[byte]Color{'f': "#6b4f36", 'p': "#9c7650", 'q': "#8d6a46", 'k': "#e0c070"}

// look is how a role dresses: shirt and hair.
type look struct{ shirt, hair, skin Color }

var looks = map[string]look{
	"main":           {"#8b5cf6", "#3f2a1d", "#f2c79b"},
	"Explore":        {"#14b8a6", "#1f2937", "#d9a066"},
	"cairn-explorer": {"#14b8a6", "#1f2937", "#d9a066"},
	"cairn-runner":   {"#f97316", "#7c4a1e", "#f2c79b"},
	"cairn-writer":   {"#ec4899", "#b8862f", "#e8b98a"},
	"Plan":           {"#3b82f6", "#5a3825", "#c68642"},
}

var spareLooks = []look{
	{"#3b82f6", "#5a3825", "#c68642"},
	{"#22c55e", "#2b1d14", "#f2c79b"},
	{"#eab308", "#1f2937", "#d9a066"},
	{"#ef4444", "#7c4a1e", "#e8b98a"},
}

// Shirt is the colour a role wears, to name it in a legend.
func Shirt(role string) Color { return lookOf(role).shirt }

func lookOf(role string) look {
	if l, ok := looks[role]; ok {
		return l
	}
	h := fnv.New32a()
	h.Write([]byte(role))
	return spareLooks[h.Sum32()%uint32(len(spareLooks))]
}

func (l look) pal() map[byte]Color {
	return map[byte]Color{'h': l.hair, 's': l.skin, 'e': "#2b2b2b", 'c': l.shirt, 'p': "#3b4256"}
}
