package qainfra

import "testing"

func TestInjectNestedVirtualization(t *testing.T) {
	const template = "module \"cluster_nodes\" {\n# __NESTED_VIRTUALIZATION_MODULE_ARGS__\n}\n"
	const enabled = "module \"cluster_nodes\" {\nworker_nested_virtualization = true\n}\n"
	const disabled = "module \"cluster_nodes\" {\n\n}\n"
	const oldTemplate = "module \"cluster_nodes\" {}\n"
	for _, tc := range []struct {
		name, content, provider, value, want string
		wantErr                              bool
	}{
		{"Kata AWS workers", template, "aws", "true", enabled, false},
		{"ordinary run", template, "aws", "", disabled, false},
		{"explicitly disabled", template, "aws", "false", disabled, false},
		{"older template without opt-in", oldTemplate, "aws", "", oldTemplate, false},
		{"other provider without opt-in", oldTemplate, "vsphere", "false", oldTemplate, false},
		{"unsupported provider", template, "vsphere", "true", "", true},
		{"missing template marker", oldTemplate, "aws", "true", "", true},
		{"invalid boolean", template, "aws", "yes", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := injectNestedVirtualization(tc.content, tc.provider, tc.value)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v; want error = %t", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("template = %q; want %q", got, tc.want)
			}
		})
	}
}
