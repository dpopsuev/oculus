package lang_test

import (
	"testing"

	"github.com/dpopsuev/oculus/lang"
)

func TestDetectLanguage_External(t *testing.T) {
	detected := lang.DetectLanguage(t.TempDir())
	_ = detected
}
