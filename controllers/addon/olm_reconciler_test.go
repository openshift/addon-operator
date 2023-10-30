package addon

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOLMReconciler_Name(t *testing.T) {
	r := &olmReconciler{}

	// The expected reconciler name
	expectedName := "olmReconciler"

	name := r.Name()

	// Verify that the reconciler name is correct
	assert.Equal(t, expectedName, name)
}
