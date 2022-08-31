package main

import (
	"fmt"
	"math/rand"
	"net/url"
	"unicode"
)

var Nouns = []string{"people", "history", "way", "art", "world", "information", "map", "two", "family",
	"government", "health", "system", "computer", "meat", "year", "thanks", "music", "person", "reading",
	"method", "data", "food", "understanding", "theory", "law", "bird", "literature", "problem", "software",
	"control", "knowledge", "power", "ability", "economics", "love", "internet", "television", "science",
	"library", "nature", "fact", "product", "idea", "temperature", "investment", "area", "society",
	"activity", "story", "industry", "media", "thing", "oven", "community", "definition", "safety",
	"quality", "development", "language", "management", "player", "variety", "video", "week",
	"security", "country", "exam", "movie", "organization", "equipment", "physics", "analysis",
	"policy", "series", "thought", "basis", "boyfriend", "direction", "strategy", "technology",
	"army", "camera", "freedom", "paper", "environment", "child", "instance", "month", "truth",
	"marketing", "university", "writing", "article", "department", "difference", "goal", "news",
	"audience", "fishing", "growth", "income", "marriage", "user", "combination", "failure", "meaning",
	"medicine", "philosophy", "teacher", "communication", "night", "chemistry", "disease", "disk",
	"energy", "nation", "road", "role", "soup", "advertising", "location", "success", "addition",
	"apartment", "education", "math", "moment", "painting", "politics", "attention",
}

var Adjectives = []string{"gaseous", "querulous", "golden", "velvety", "altruistic", "ignorant", "rundown",
	"black", "glass", "personal", "portly", "damaged", "nutty", "attentive", "sparkling", "novel", "scientific",
	"wealthy", "monstrous", "turbulent", "tender", "spiffy", "tricky", "humiliating", "bouncy", "glaring",
	"angry", "shameless", "unfinished", "upbeat", "vague", "mealy", "double", "writhing", "intrepid",
	"stupid", "bewitched", "devoted", "rowdy", "phony", "big", "blond", "rapid", "hurtful", "enormous",
	"curly", "knobby", "large", "euphoric", "prudent", "disguised", "tame", "unsung", "mortified", "eager",
	"friendly", "huge", "simplistic", "canine", "slippery", "puzzling", "slow", "downright", "grave",
	"late", "outrageous", "shiny", "blind", "other", "astonishing", "cooperative", "unacceptable",
	"forked", "adventurous", "avaricious", "inferior", "thin", "kooky", "utter", "complete", "merry",
	"hospitable", "classic", "light", "perky", "scented", "advanced", "deficient", "growing", "favorable",
	"grotesque", "pure", "talkative", "optimal", "true", "breakable", "genuine", "expert", "showy",
	"comfortable", "yawning", "impossible", "helpful", "passionate", "speedy", "disgusting", "rural",
	"bare", "demanding", "enchanting", "plump", "incompatible", "frilly", "humongous", "noteworthy",
	"flat", "intelligent", "angry", "faraway", "harsh", "partial", "wry", "stormy", "loose", "wordy",
	"coordinated", "icky", "imaginary", "immaculate", "gloomy", "informed", "slim", "steel",
	"ordinary", "gruesome", "trim", "hidden",
}

var currentUrls map[string]int

// Cap the first letter of the word
func cap(word string) string {
	r := []rune(word)
	r[0] = unicode.ToUpper(r[0])
	word = string(r)
	return word
}

// GenUrls
// Generates ADI and an account (as provided)
// We use a random matching of adjs and nouns, so we use a map to ensure we have not
// returned a pair before.
func GenUrls(account string) (ADI, URL *url.URL) {
	if currentUrls == nil {
		currentUrls = make(map[string]int) // allocate the duplicate map
		rand.Seed(1971)                    // make sure we return the same data for calls in the same order
	}
	i := rand.Int() % len(Adjectives)
	j := rand.Int() % len(Nouns)
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
