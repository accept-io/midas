package config

import "testing"

func TestExplorerUnauthenticated(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want bool
	}{
		{
			name: "explorer_enabled_no_iam_no_oidc_warns",
			cfg: func() Config {
				c := DefaultConfig()
				c.Server.ExplorerEnabled = true
				c.LocalIAM.Enabled = false
				c.PlatformOIDC.Enabled = false
				return c
			}(),
			want: true,
		},
		{
			name: "explorer_enabled_local_iam_active_no_warn",
			cfg: func() Config {
				c := DefaultConfig()
				c.Server.ExplorerEnabled = true
				c.LocalIAM.Enabled = true
				return c
			}(),
			want: false,
		},
		{
			name: "explorer_enabled_oidc_active_no_warn",
			cfg: func() Config {
				c := DefaultConfig()
				c.Server.ExplorerEnabled = true
				c.LocalIAM.Enabled = false
				c.PlatformOIDC.Enabled = true
				return c
			}(),
			want: false,
		},
		{
			name: "headless_no_warn",
			cfg: func() Config {
				c := DefaultConfig()
				c.Server.Headless = true
				c.Server.ExplorerEnabled = false
				c.LocalIAM.Enabled = false
				return c
			}(),
			want: false,
		},
		{
			name: "explorer_disabled_no_warn",
			cfg: func() Config {
				c := DefaultConfig()
				c.Server.ExplorerEnabled = false
				c.LocalIAM.Enabled = false
				return c
			}(),
			want: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := explorerUnauthenticated(c.cfg)
			if got != c.want {
				t.Errorf("explorerUnauthenticated: want %v, got %v", c.want, got)
			}
		})
	}
}
