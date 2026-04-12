package unit_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// ── Pinyin Conversion (pure function tests) ───────────────────────────────────

// pinyinConvert simulates the toPinyin logic using a static map.
// This mirrors SearchService.toPinyin without a database dependency.
func pinyinConvert(text string, mapping map[string]string) string {
	if text == "" {
		return ""
	}
	var result []string
	for _, r := range text {
		char := string(r)
		if pinyin, ok := mapping[char]; ok {
			result = append(result, pinyin)
		} else if r > 127 {
			continue
		} else {
			result = append(result, char)
		}
	}
	return strings.Join(result, "")
}

func TestPinyinConvert_EmptyString(t *testing.T) {
	m := map[string]string{"教": "jiao", "学": "xue"}
	assert.Equal(t, "", pinyinConvert("", m))
}

func TestPinyinConvert_AllASCII(t *testing.T) {
	m := map[string]string{"教": "jiao"}
	assert.Equal(t, "hello", pinyinConvert("hello", m))
}

func TestPinyinConvert_ChineseToJiaoxue(t *testing.T) {
	m := map[string]string{"教": "jiao", "学": "xue"}
	result := pinyinConvert("教学", m)
	assert.Equal(t, "jiaoxue", result)
}

func TestPinyinConvert_MixedContent(t *testing.T) {
	m := map[string]string{"数": "shu", "学": "xue"}
	result := pinyinConvert("数学 math", m)
	assert.Contains(t, result, "shuxue")
	assert.Contains(t, result, "math")
}

func TestPinyinConvert_UnmappedChinese_Skipped(t *testing.T) {
	m := map[string]string{"教": "jiao"} // "学" not in map
	result := pinyinConvert("教学", m)
	assert.Equal(t, "jiao", result) // "学" skipped
}

// ── Synonym Expansion (pure function test) ────────────────────────────────────

// expandQuery simulates synonym expansion with a static synonym list.
func expandQuery(query string, groups []struct{ canonical string; synonyms []string }) []string {
	q := strings.ToLower(strings.TrimSpace(query))
	seen := map[string]bool{q: true}
	var expanded []string
	for _, g := range groups {
		if g.canonical == q || containsStr(g.synonyms, q) {
			if !seen[g.canonical] {
				seen[g.canonical] = true
				expanded = append(expanded, g.canonical)
			}
			for _, s := range g.synonyms {
				if !seen[s] {
					seen[s] = true
					expanded = append(expanded, s)
				}
			}
		}
	}
	return expanded
}

func containsStr(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func TestExpandQuery_MatchCanonical(t *testing.T) {
	groups := []struct{ canonical string; synonyms []string }{
		{canonical: "mathematics", synonyms: []string{"math", "maths", "algebra"}},
	}
	expanded := expandQuery("mathematics", groups)
	assert.Contains(t, expanded, "math")
	assert.Contains(t, expanded, "maths")
	assert.Contains(t, expanded, "algebra")
}

func TestExpandQuery_MatchSynonym(t *testing.T) {
	groups := []struct{ canonical string; synonyms []string }{
		{canonical: "mathematics", synonyms: []string{"math", "maths"}},
	}
	expanded := expandQuery("math", groups)
	assert.Contains(t, expanded, "mathematics")
	assert.Contains(t, expanded, "maths")
}

func TestExpandQuery_NoMatch(t *testing.T) {
	groups := []struct{ canonical string; synonyms []string }{
		{canonical: "mathematics", synonyms: []string{"math"}},
	}
	expanded := expandQuery("physics", groups)
	assert.Empty(t, expanded)
}

func TestExpandQuery_CaseInsensitive(t *testing.T) {
	groups := []struct{ canonical string; synonyms []string }{
		{canonical: "programming", synonyms: []string{"coding", "development"}},
	}
	expanded := expandQuery("PROGRAMMING", groups)
	assert.Contains(t, expanded, "coding")
	assert.Contains(t, expanded, "development")
}

func TestExpandQuery_DeduplicatesTerms(t *testing.T) {
	groups := []struct{ canonical string; synonyms []string }{
		{canonical: "intro", synonyms: []string{"introduction", "basics"}},
		{canonical: "introduction", synonyms: []string{"intro", "fundamentals"}},
	}
	expanded := expandQuery("intro", groups)
	// Should not have duplicates
	seen := map[string]int{}
	for _, e := range expanded {
		seen[e]++
	}
	for term, count := range seen {
		assert.Equal(t, 1, count, "term %q appears more than once", term)
	}
}
