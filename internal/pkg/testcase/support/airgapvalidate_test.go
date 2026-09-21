package support

import "testing"

func TestAssertEgressBlocked(t *testing.T) {
	cases := map[string]bool{
		"140.82.112.3 rc=28\n1.1.1.1 rc=28": true,
		"140.82.112.3 rc=7\n1.1.1.1 rc=28":  false, // refused = something answered
		"140.82.112.3 rc=0\n1.1.1.1 rc=28":  false, // reached github
		"140.82.112.3 rc=60\n1.1.1.1 rc=28": false, // TLS handshake happened = connectivity
		"140.82.112.3 rc=127":               false, // curl missing: inconclusive, never a pass
		"":                                  false,
	}
	for in, ok := range cases {
		err := assertEgressBlocked(in)
		if ok && err != nil {
			t.Errorf("%q: unexpected error %v", in, err)
		}
		if !ok && err == nil {
			t.Errorf("%q: expected an error", in)
		}
	}
}
