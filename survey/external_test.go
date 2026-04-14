package survey_test

import (
	"testing"

	"github.com/dpopsuev/oculus/v3/survey"
)

func TestDetectLanguage_External(t *testing.T) {
	lang := survey.DetectLanguage(t.TempDir())
	_ = lang
}
