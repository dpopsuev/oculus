package patterns

import "fmt"

// NewWidget creates a widget by type name.
func NewWidget(typeName string) string {
	return fmt.Sprintf("widget:%s", typeName)
}

