package customflag

import (
	"flag"
	"fmt"
	"os"
)

// FillFromEnv sets every flag in flagToEnv that was not given on the command line
// from its environment variable, so config/.env can carry suite inputs; flags win.
func FillFromEnv(fs *flag.FlagSet, flagToEnv map[string]string) error {
	given := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { given[f.Name] = true })

	for name, env := range flagToEnv {
		value, ok := os.LookupEnv(env)
		if given[name] || !ok || value == "" {
			continue
		}
		if err := fs.Set(name, value); err != nil {
			return fmt.Errorf("set -%s from %s: %w", name, env, err)
		}
	}

	return nil
}
