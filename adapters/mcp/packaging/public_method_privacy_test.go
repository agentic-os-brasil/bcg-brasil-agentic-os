package packaging

import (
	"regexp"
	"testing"
)

// Match both ordinary Windows separators and the escaped form in Markdown.
var privateSourceLocation = regexp.MustCompile(`(?:/Users/[^/\s]+/|/home/[^/\s]+/|(?i:[a-z]:\\+Users\\+[^\\\r\n]+))`)

func assertPublicMethodPrivacy(t *testing.T, files map[string][]byte) {
	t.Helper()
	for path, body := range files {
		if privateSourceLocation.Match(body) {
			t.Errorf("private user source location in emitted resource %s", path)
		}
	}
}

func TestPublicMethodResourcesExcludePrivateSourceLocations(t *testing.T) {
	for _, namespace := range []string{"", "maestro-product"} {
		files, err := methodPayload(namespace)
		if err != nil {
			t.Fatal(err)
		}
		assertPublicMethodPrivacy(t, files)
	}
}

func TestPrivateSourceLocationMatchesPlatformForms(t *testing.T) {
	for _, example := range []string{`C:\Users\example\Private\source.pdf`, `C:\\Users\\example\\Private\\source.pdf`, `/Users/example/Private/source.pdf`, `/home/example/private/source.pdf`} {
		if !privateSourceLocation.MatchString(example) {
			t.Error("private source pattern missed a platform form")
		}
	}
	for _, example := range []string{`source.pdf`, `resources/source.pdf`, `maestro://methods/catalog.json`, `/users/${id}/orders`} {
		if privateSourceLocation.MatchString(example) {
			t.Error("public reference incorrectly marked private")
		}
	}
}
