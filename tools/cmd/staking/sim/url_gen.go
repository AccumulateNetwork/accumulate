package sim

import (
	"fmt"
	"unicode"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/smt/common"
)

var Nouns = []string{
	"account", "middle", "act", "milk", "adjustment", "mind", "advertisement", "mine",
	"agreement", "minute", "air", "mist", "amount", "money", "amusement", "month", "animal",
	"morning", "answer", "mother", "apparatus", "motion", "approval", "mountain",
	"argument", "move", "art", "music", "attack", "name", "attempt", "nation", "attention",
	"need", "attraction", "news", "authority", "night", "back", "noise", "balance",
	"note", "base", "number", "behavior", "observation", "belief", "offer", "birth",
	"oil", "bit", "operation", "bite", "opinion", "blood", "order", "blow",
	"organization", "body", "ornament", "brass", "owner", "bread", "page", "breath", "pain",
	"brother", "paint", "building", "paper", "burn", "part", "burst", "paste", "business",
	"payment", "butter", "peace", "canvas", "person", "care", "place", "cause", "plant",
	"chalk", "play", "chance", "pleasure", "change", "point", "cloth", "poison", "coal",
	"polish", "color", "porter", "comfort", "position", "committee", "powder", "company",
	"power", "comparison", "price", "competition", "print", "condition", "process",
	"connection", "produce", "control", "profit", "cook", "property", "copper", "prose",
	"copy", "protest", "cork", "pull", "copy", "punishment", "cough", "purpose", "country",
	"push", "cover", "quality", "crack", "question", "credit", "rain", "crime", "range",
	"crush", "rate", "cry", "ray", "current", "reaction", "curve", "reading", "damage",
	"reason", "danger", "record", "daughter", "regret", "day", "relation", "death",
	"religion", "debt", "representative", "decision", "request", "degree", "respect",
	"design", "rest", "desire", "reward", "destruction", "rhythm", "detail", "rice",
	"development", "river", "digestion", "road", "direction", "roll", "discovery", "room",
	"discussion", "rub", "disease", "rule", "disgust", "run", "distance", "salt",
	"distribution", "sand", "division", "scale", "doubt", "science", "drink", "sea",
	"driving", "seat", "dust", "secretary", "earth", "selection", "edge", "self",
	"education", "sense", "effect", "servant", "end", "error", "shade", "event", "shake",
	"example", "shame", "exchange", "shock", "existence", "side", "expansion", "sign",
	"experience", "silk", "expert", "silver", "fact", "sister", "fall", "size", "family",
	"sky", "father", "sleep", "fear", "slip", "feeling", "slope", "fiction", "smash",
	"field", "smell", "fight", "smile", "fire", "smoke", "flame", "sneeze", "flight", "snow",
	"flower", "soap", "fold", "society", "food", "son", "force", "song", "form", "sort",
	"friend", "sound", "front", "soup", "fruit", "space", "glass", "stage", "gold", "start",
	"government", "statement", "grain", "steam", "grass", "steel", "grip", "step", "group",
	"stitch", "growth", "stone", "guide", "stop", "harbor", "story", "harmony", "stretch",
	"hate", "structure", "hearing", "substance", "heat", "sugar", "help", "suggestion",
	"history", "summer", "hole", "support", "hope", "surprise", "hour", "swim", "humor",
	"system", "ice", "talk", "idea", "taste", "impulse", "tax", "increase", "teaching",
	"industry", "tendency", "ink", "test", "insect", "theory", "instrument", "thing",
	"insurance", "thought", "interest", "thunder", "invention", "time", "iron", "tin",
	"jelly", "top", "join", "touch", "journey", "trade", "judge", "transport", "jump",
	"trick", "kick", "trouble", "kiss", "turn", "knowledge", "twist", "land", "unit",
	"language", "use", "laugh", "value", "low", "verse", "lead", "vessel", "learning",
	"view", "leather", "voice", "letter", "walk", "level", "war", "lift", "wash",
	"light", "waste", "limit", "water", "linen", "wave", "liquid", "wax", "list", "way",
	"look", "weather", "loss", "week", "love", "weight", "machine", "wind", "man", "wine",
	"manager", "winter", "mark", "woman", "market", "wood", "mass", "wool", "meal", "word",
	"measure", "work", "meat", "wound", "meeting", "writing", "memory", "year", "metal",
	"angle", "knee", "ant", "knife", "apple", "knot", "arch", "leaf", "arm", "leg", "army",
	"library", "baby", "line", "bag", "lip", "ball", "lock", "band", "map", "basin", "match",
	"basket", "monkey", "bath", "moon", "bed", "mouth", "bee", "muscle", "bell", "nail",
	"berry", "neck", "bird", "needle", "blade", "nerve", "board", "net", "boat", "nose",
	"bone", "nut", "book", "office", "boot", "orange", "bottle", "oven", "box", "parcel",
	"boy", "pen", "brain", "pencil", "brake", "picture", "branch", "pig", "brick", "pin",
	"bridge", "pipe", "brush", "plane", "bucket", "plate", "bulb", "plough", "button",
	"pocket", "cake", "pot", "camera", "potato", "card", "prison", "carriage", "pump",
	"cart", "rail", "cat", "rat", "chain", "receipt", "cheese", "ring", "chess", "rod",
	"chin", "roof", "church", "root", "circle", "sail", "clock", "school", "cloud",
	"scissors", "coat", "screw", "collar", "seed", "comb", "sheep", "cord", "shelf", "cow",
	"ship", "cup", "shirt", "curtain", "shoe", "cushion", "skin", "dog", "skirt", "door",
	"snake", "drain", "sock", "drawer", "spade", "dress", "sponge", "drop", "spoon", "ear",
	"spring", "egg", "square", "engine", "stamp", "eye", "star", "face", "station", "farm",
	"stem", "feather", "stick", "finger", "stocking", "fish", "stomach", "flag", "store",
	"floor", "street", "fly", "sun", "foot", "table", "fork", "tail", "fowl", "thread",
	"frame", "throat", "garden", "thumb", "girl", "ticket", "glove", "toe", "goat", "tongue",
	"gun", "tooth", "hair", "town", "hammer", "train", "hand", "tray", "hat", "tree", "head",
	"trousers", "heart", "umbrella", "hook", "wall", "horn", "watch", "horse", "wheel",
	"hospital", "whip", "house", "whistle", "island", "window", "jewel", "wing", "kettle",
	"wire", "key", "worm",
}

var Adjectives = []string{"Abundant", "Accurate", "Addicted", "Adorable", "Adventurous", "Afraid", "Aggressive",
	"Alcoholic", "Alert", "Aloof", "Ambitious", "Ancient", "Angry", "Animated", "Annoying",
	"Anxious", "Arrogant", "Ashamed", "Attractive", "Auspicious", "Awesome", "Awful",
	"Abactinal", "Abandoned", "Abashed", "Abatable", "Abatic", "Abaxial", "Abbatial",
	"Abbreviated", "Abducent", "Abducting", "Aberrant", "Abeyant", "Abhorrent", "Abiding",
	"Abient", "Bad", "Bashful", "Beautiful", "Belligerent", "Beneficial", "Best", "Big",
	"Bitter", "Bizarre", "Black", "Blue", "Boring", "Brainy", "Bright", "Broad", "Broken",
	"Busy", "Barren", "Barricaded", "Barytic", "Basal", "Basaltic", "Baseborn", "Based",
	"Baseless", "Basic", "Bathyal", "Battleful", "Battlemented", "Batty", "Batwing",
	"Bias", "Calm", "Capable", "Careful", "Careless", "Caring", "Cautious", "Charming",
	"Cheap", "Cheerful", "Chubby", "Clean", "Clever", "Clumsy", "Cold", "Colorful",
	"Comfortable", "Concerned", "Confused", "Crowded", "Cruel", "Curious", "Curly", "Cute",
	"Damaged", "Dangerous", "Dark", "Deep", "Defective", "Delicate", "Delicious",
	"Depressed", "Determined", "Different", "Dirty", "Disgusting", "Dry", "Dusty", "Daft",
	"Daily", "Dainty", "Damn", "Damning", "Damp", "Dampish", "Darkling", "Darned",
	"Dauntless", "Daylong", "Early", "Educated", "Efficient", "Elderly", "Elegant",
	"Embarrassed", "Empty", "Encouraging", "Enthusiastic", "Excellent", "Exciting",
	"Expensive", "Fabulous", "Fair", "Faithful", "Famous", "Fancy", "Fantastic", "Fast",
	"Fearful", "Fearless", "Fertile", "Filthy", "Foolish", "Forgetful", "Friendly",
	"Funny", "Gentle", "Glamorous", "Glorious", "Gorgeous", "Graceful", "Grateful",
	"Great", "Greedy", "Green", "Handsome", "Happy", "Harsh", "Healthy", "Heavy", "Helpful",
	"Hilarious", "Historical", "Horrible", "Hot", "Huge", "Humorous", "Hungry", "Ignorant",
	"Illegal", "Imaginary", "Impolite", "Important", "Impossible", "Innocent",
	"Intelligent", "Interesting", "Jealous", "Jolly", "Juicy", "Juvenile", "Kind", "Large",
	"Legal", "Light", "Literate", "Little", "Lively", "Lonely", "Loud", "Lovely", "Lucky",
	"Macho", "Magical", "Magnificent", "Massive", "Mature", "Mean", "Messy", "Modern",
	"Narrow", "Nasty", "Naughty", "Nervous", "New", "Noisy", "Nutritious", "Obedient",
	"Obese", "Obnoxious", "Old", "Overconfident", "Peaceful", "Pink", "Polite", "Poor",
	"Powerful", "Precious", "Pretty", "Proud", "Quick", "Quiet", "Rapid", "Rare", "Red",
	"Remarkable", "Responsible", "Rich", "Romantic", "Royal", "Rude", "Scintillating",
	"Secretive", "Selfish", "Serious", "Sharp", "Shiny", "Shocking", "Short", "Shy",
	"Silly", "Sincere", "Skinny", "Slim", "Slow", "Small", "Soft", "Spicy", "Spiritual",
	"Splendid", "Strong", "Successful", "Sweet", "Talented", "Tall", "Tense", "Terrible",
	"Terrific", "Thick", "Thin", "Tiny", "Tactful", "Tailor-made", "Take-charge",
	"Tangible", "Tasteful", "Tasty", "Teachable", "Teeming", "Tempean", "Temperate",
	"Tenable", "Tenacious", "Tender", "Tender-hearted", "Terrific", "Testimonial",
	"Thankful", "Thankworthy", "Therapeutic", "Thorough", "Thoughtful", "Ugly", "Unique",
	"Untidy", "Upset", "Victorious", "Violent", "Vulgar", "Warm", "Weak", "Wealthy", "Wide",
	"Wise", "Witty", "Wonderful", "Worried", "Young", "Youthful", "Zealous",
}

var currentUrls map[string]int

// Cap the first letter of the word
func cap(word string) string {
	r := []rune(word)
	r[0] = unicode.ToUpper(r[0])
	word = string(r)
	return word
}

var GenRH common.RandHash

// GenUrls
// Generates ADI and an account (as provided)
// We use a random matching of adjs and nouns, so we use a map to ensure we have not
// returned a pair before.
func GenUrls(account string) (ADI, URL *url.URL) {
	if currentUrls == nil {
		currentUrls = make(map[string]int) // allocate the duplicate map
	}
	i := GenRH.GetRandInt64() % int64(len(Adjectives))
	j := GenRH.GetRandInt64() % int64(len(Nouns))
	a := "acc://" + cap(Adjectives[i]) + cap(Nouns[j]) + ".acme"
	u := fmt.Sprintf("%s/%s", a, account)
	if currentUrls[u] == 1 {
		return GenUrls(account)
	}
	currentUrls[u] = 1
	ADI, err1 := url.Parse(a)
	URL, err2 := url.Parse(u)
	if err1 != nil || err2 != nil {
		panic(fmt.Sprintf("Found error(s) %v %v ", err1, err2))
	}
	return ADI, URL
}

// GenAccount
// Same as GenUrl but doesn't return the ADI
func GenAccount(account string) (Url *url.URL) {
	_, u := GenUrls(account)
	return u
}
