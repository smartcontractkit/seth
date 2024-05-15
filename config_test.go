package seth_test

import (
	"github.com/smartcontractkit/seth"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestValidateConfigKeyfile(t *testing.T) {
	type tc struct {
		name string
		cfg  seth.Config
		err  string
	}

	var one int64 = 1

	tcs := []tc{
		{
			name: "without keyfile",
			cfg: seth.Config{
				Network: &seth.Network{},
			},
			err: "",
		},
		{
			name: "without keyfile with file path set",
			cfg: seth.Config{
				Network:     &seth.Network{},
				KeyFilePath: "keyfile_test.toml",
			},
			err: "",
		},
		{
			name: "valid keyfile with file source",
			cfg: seth.Config{
				Network:       &seth.Network{},
				KeyFileSource: seth.KeyFileSourceFile,
				KeyFilePath:   "keyfile_test.toml",
			},
			err: "",
		},
		{
			name: "valid keyfile with env var source",
			cfg: seth.Config{
				Network:       &seth.Network{},
				KeyFileSource: seth.KeyFileSourceBase64EnvVar,
			},
			err: "",
		},
		{
			name: "invalid keyfile with env var source, but no env var set",
			cfg: seth.Config{
				Network:       &seth.Network{},
				KeyFileSource: seth.KeyFileSourceBase64EnvVar,
			},
			err: "KeyFileSource is set to 'base64-env-var' but the environment variable 'KEYFILE_BASE64' is not set",
		},
		{
			name: "invalid keyfile with file source, no path set",
			cfg: seth.Config{
				Network:       &seth.Network{},
				KeyFileSource: seth.KeyFileSourceFile,
			},
			err: "KeyFileSource is set to 'file' but the path to the key file is not set",
		},
		{
			name: "invalid keyfile source",
			cfg: seth.Config{
				Network:       &seth.Network{},
				KeyFileSource: "bla bla",
			},
			err: "KeyFileSource must be either empty (disabled) or one of: 'file', 'base64_env'",
		},
		{
			name: "keyfile set to 'file' with ephemeral addresses",
			cfg: seth.Config{
				Network:        &seth.Network{},
				KeyFileSource:  seth.KeyFileSourceFile,
				KeyFilePath:    "keyfile_test.toml",
				EphemeralAddrs: &one,
			},
			err: "KeyFileSource is set to 'file' and ephemeral addresses are enabled, please disable ephemeral addresses or the keyfile usage. You cannot use both modes at the same time",
		},
		{
			name: "keyfile set to 'base64_env' with ephemeral addresses",
			cfg: seth.Config{
				Network:        &seth.Network{},
				KeyFileSource:  seth.KeyFileSourceBase64EnvVar,
				EphemeralAddrs: &one,
			},
			err: "KeyFileSource is set to 'base64_env' and ephemeral addresses are enabled, please disable ephemeral addresses or the keyfile usage. You cannot use both modes at the same time",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			if tc.cfg.KeyFileSource == seth.KeyFileSourceBase64EnvVar && tc.err == "" {
				err := os.Setenv(seth.KEYFILE_BASE64_ENV_VAR, "dGVzdA==")
				require.NoError(t, err, "failed to set env var")

				defer func() {
					_ = os.Unsetenv(seth.KEYFILE_BASE64_ENV_VAR)
				}()
			}
			err := seth.ValidateConfig(&tc.cfg)
			if tc.err != "" {
				require.EqualError(t, err, tc.err, "expected error")
			} else {
				require.NoError(t, err, "expected no error")
			}
		})
	}
}

func TestDecodeKeyfileFromBase64(t *testing.T) {
	base64edKeyfile := "W1trZXlzXV0KcHJpdmF0ZV9rZXkgPSAnZDZlZjhlZGM4MmNjOGFlYThmNDM3NGQ3MWU0NjEzYTE0YzI4ZTExMGU1MThmNWExMDNhM2Q5NWI1OTk0ZmYzZicKYWRkcmVzcyA9ICcweEM1MkRBRTY3YTgwRDI3YTE5ODMyYTZmZjg5MzlhOUI4NjVkZTZlYjcnCmZ1bmRzID0gJzE5OTg5OTk5NzkwMDAwMDAwMDAwMDAnCgpbW2tleXNdXQpwcml2YXRlX2tleSA9ICdiMWE2NzMwNDJkOGY2ZmNmNjQ1YjQwMDRkMTI5Zjc5NDNkZjVlYjY0Yjg2ZTg2MzA5OTkwZTdlMGI5YTE5ZTdlJwphZGRyZXNzID0gJzB4YjZDM2Y5QzYyMEY5NTkyMzQ1ZGY2RjY0OTBGM2NBQWYyM2JFRTM0MScKZnVuZHMgPSAnMTk5ODk5OTk3OTAwMDAwMDAwMDAwMCcKCltba2V5c11dCnByaXZhdGVfa2V5ID0gJzViYTVjMzdjNGY1NTg4MWRmOWQ0YTdlYzllYzdhMTVjODAxZWI4NmJmZmY5MDU5YTcxNjM1YTEyMGI4OTA2NDAnCmFkZHJlc3MgPSAnMHg1MzdiQUE2YzVmZTJBNTJjZjYxMjNkNEE5ZjgxMUJmZDRCMzZiMTRlJwpmdW5kcyA9ICcxOTk4OTk5OTc5MDAwMDAwMDAwMDAwJwoKW1trZXlzXV0KcHJpdmF0ZV9rZXkgPSAnMGU2YzVjZGZjNmExYzQ4MjFmZTM3ZTk0ODY1YWNmOWE4ZGZmOTU2ZDNmZWY2MjQyMTQzYjE2ODU4MmVkYjI0OScKYWRkcmVzcyA9ICcweDBkZWNhNWZDMDIyMzQ0RDUxNTNiRGQxMzhCQWM1MUNhOGQyNUNhM2YnCmZ1bmRzID0gJzE5OTg5OTk5NzkwMDAwMDAwMDAwMDAnCgpbW2tleXNdXQpwcml2YXRlX2tleSA9ICc0ZGU2ZGVmNzc2MzE5ODYzZGI1NTg0MDc3MjNlYTViNmQ1NzBiZjdjMjQwNTBjYmU5OTVhOGU5NzA0NjkwZmJmJwphZGRyZXNzID0gJzB4MkE5REI1MWIzYjMwNTQ2ZjhkMUY3M2NjQkYxMjVBMjAwMmY0NTcyNCcKZnVuZHMgPSAnMTk5ODk5OTk3OTAwMDAwMDAwMDAwMCcKCltba2V5c11dCnByaXZhdGVfa2V5ID0gJzIyMDQ5NmU1MWE0ZDEyZmRjYmEwNmY0ZDYwMGYwYTgzMzc4NDU2MmU0NjljMDMzOTRlZWMzNmUyYTNjZmY2MTgnCmFkZHJlc3MgPSAnMHgzNjM1NzJkRmNFRjhmZWE0QWI5ZjNiYzc0M2ZDNDg3YUQ0YzMzMzJGJwpmdW5kcyA9ICcxOTk4OTk5OTc5MDAwMDAwMDAwMDAwJwoKW1trZXlzXV0KcHJpdmF0ZV9rZXkgPSAnZTZkOTZkZGMwMTZkNzc4NTMzMmM4NmRhNDI0MDI5ZjVhMTFlNDNjN2MzNDUxZTAxNjM1NDI4NDIxMTRjMGYxNicKYWRkcmVzcyA9ICcweDc0MUJjMjMwQWNFQzY2MDE0ZEJlRUUyZkI2RDVBOEU5YkI4ODFFMUMnCmZ1bmRzID0gJzE5OTg5OTk5NzkwMDAwMDAwMDAwMDAnCgpbW2tleXNdXQpwcml2YXRlX2tleSA9ICc2MTNkZjBhNjc2YTI1YmNiMzkyMDA4NmZhZjU2Mzc5NjM3OGUxMGY0NzQxN2RhNTVjZWE0YTBmNWE4NDllNzVmJwphZGRyZXNzID0gJzB4NTY0MTAyOTAyQTdCNjhFOTVDOTdkMTNhRmQ4YTE5YzIwQTVjMDVGNycKZnVuZHMgPSAnMTk5ODk5OTk3OTAwMDAwMDAwMDAwMCcKCltba2V5c11dCnByaXZhdGVfa2V5ID0gJzZkNjAwNzMwY2YxMTRhNWYwNDFiMTFhNTliMTIwOTA3YWNlMDU1ZGE3MDY2YTE3NmQzNmUxOTQwM2M2YjlhMjknCmFkZHJlc3MgPSAnMHg1OTMxOGU2OTc5M0JlODI5RTg4NjkxNjJBNjhkM0JjMDc4NGIwQmVDJwpmdW5kcyA9ICcxOTk4OTk5OTc5MDAwMDAwMDAwMDAwJwoKW1trZXlzXV0KcHJpdmF0ZV9rZXkgPSAnMTk5ZWRhMDU2NGI5MWU4YmYxZjdhZGQzNTE3MGY3NjVhNTgxNGE0M2M4ZjIwNjk0Y2E1MjhjNjdmNThjYjRjZScKYWRkcmVzcyA9ICcweEE5OWRFZTVFODgzNTIxQTFmNTQ4NTI2NkJiQjU0NDg0MEVlMDcxZjAnCmZ1bmRzID0gJzE5OTg5OTk5NzkwMDAwMDAwMDAwMDAnCg=="
	err := os.Setenv(seth.KEYFILE_BASE64_ENV_VAR, base64edKeyfile)
	require.NoError(t, err, "failed to set env var")

	defer func() {
		_ = os.Unsetenv(seth.KEYFILE_BASE64_ENV_VAR)
	}()

	cfg, err := seth.ReadConfig()
	require.NoError(t, err, "failed to read config")

	cfg.KeyFileSource = seth.KeyFileSourceBase64EnvVar

	c, err := seth.NewClientWithConfig(cfg)
	require.NoError(t, err, "failed to initialise seth")

	require.Equal(t, 11, len(c.Addresses), "expected 10 addresses")
	require.Equal(t, 11, len(c.PrivateKeys), "expected 10 private keys")
}
