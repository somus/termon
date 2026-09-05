package expedition

import "fmt"

var supportPools = map[string][]string{
	"rootkit":      {"sproutware", "emberbyte"},
	"sproutware":   {"thornpatch", "emberbyte"},
	"thornpatch":   {"mossmuff", "emberbyte"},
	"mossmuff":     {"rootanami", "emberbyte"},
	"rootanami":    {"rootkit", "emberbyte"},
	"emberbyte":    {"scorchip", "aquabit"},
	"cindernode":   {"wickware", "aquabit"},
	"scorchip":     {"emberbyte", "aquabit"},
	"wickware":     {"cindernode", "aquabit"},
	"aquabit":      {"gushkit", "rootkit"},
	"flowcell":     {"mistcache", "rootkit"},
	"gushkit":      {"splashscreen", "rootkit"},
	"mistcache":    {"flowcell", "rootkit"},
	"splashscreen": {"aquabit", "rootkit"},
	"zaplet":       {"joulpup", "spamlet"},
	"joulpup":      {"amperent", "spamlet"},
	"amperent":     {"surgetail", "spamlet"},
	"surgetail":    {"zaplet", "spamlet"},
	"spamlet":      {"bloatware", "chippunk"},
	"bloatware":    {"wormate", "chippunk"},
	"wormate":      {"spamlet", "chippunk"},
	"chippunk":     {"coghound", "zaplet"},
	"coghound":     {"servoboar", "zaplet"},
	"servoboar":    {"chippunk", "zaplet"},
}

var supportThemes = map[string]string{
	"rootkit": "organic siblings · thermal counterplay", "sproutware": "organic siblings · thermal counterplay",
	"thornpatch": "organic siblings · thermal counterplay", "mossmuff": "organic siblings · thermal counterplay",
	"rootanami": "organic siblings · thermal counterplay",
	"emberbyte": "thermal siblings · coolant counterplay", "cindernode": "thermal siblings · coolant counterplay",
	"scorchip": "thermal siblings · coolant counterplay", "wickware": "thermal siblings · coolant counterplay",
	"aquabit": "coolant siblings · organic counterplay", "flowcell": "coolant siblings · organic counterplay",
	"gushkit": "coolant siblings · organic counterplay", "mistcache": "coolant siblings · organic counterplay",
	"splashscreen": "coolant siblings · organic counterplay",
	"zaplet":       "current siblings · virus counterplay", "joulpup": "current siblings · virus counterplay",
	"amperent": "current siblings · virus counterplay", "surgetail": "current siblings · virus counterplay",
	"spamlet": "virus siblings · silicon counterplay", "bloatware": "virus siblings · silicon counterplay",
	"wormate":  "virus siblings · silicon counterplay",
	"chippunk": "silicon siblings · current counterplay", "coghound": "silicon siblings · current counterplay",
	"servoboar": "silicon siblings · current counterplay",
}

// SupportPool returns curated base-stage Species for Preparation Encounters.
func SupportPool(family string) []string {
	if p, ok := supportPools[family]; ok {
		out := make([]string, len(p))
		copy(out, p)
		return out
	}
	return nil
}

// SupportTheme is the player-facing support-pool label on Signal Board cards.
func SupportTheme(family string) string {
	return supportThemes[family]
}

// DrawPreps picks two distinct non-target pool entries deterministically from seed.
func DrawPreps(pool []string, target string, seed uint64) (prep1, prep2 string, err error) {
	var candidates []string
	for _, s := range pool {
		if s != target {
			candidates = append(candidates, s)
		}
	}
	if len(candidates) < 2 {
		return "", "", fmt.Errorf("expedition: support pool too small for %q", target)
	}
	i0 := int(seed % uint64(len(candidates))) //nolint:gosec // pool length is tiny
	prep1 = candidates[i0]
	rest := append(append([]string(nil), candidates[:i0]...), candidates[i0+1:]...)
	i1 := int((seed / 9973) % uint64(len(rest))) //nolint:gosec // pool length is tiny
	prep2 = rest[i1]
	return prep1, prep2, nil
}
