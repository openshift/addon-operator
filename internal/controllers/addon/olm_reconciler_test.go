package addon

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOLMReconciler_Name(t *testing.T) {
	// Create an instance of the olmReconciler
	r := &olmReconciler{}

	// Call the Name method and get the result
	name := r.Name()

	// Assert that the returned name matches the expected value
	expectedName := OLM_RECONCILER_NAME
	require.Equal(t, expectedName, name, "Unexpected reconciler name")
}