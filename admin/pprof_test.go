package admin

import (
	"fmt"
	"os"
	"testing"
)

func TestAdmin__profileEnabled(t *testing.T) {
	cases := map[string]bool{
		// enable
		"yes":    true,
		" true ": true,
		"":       true,
		// disable
		"no":       false,
		"jsadlsaj": false,
	}
	for value, enabled := range cases {
		os.Setenv("PPROF_TESTING_VALUE", fmt.Sprintf("%v", enabled))
		if v := profileEnabled("TESTING_VALUE"); v != enabled {
			t.Errorf("value=%q, got=%v, expected=%v", value, v, enabled)
		}
	}
}
