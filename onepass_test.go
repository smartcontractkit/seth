package seth_test

import (
	"fmt"
	"github.com/smartcontractkit/seth"
	"github.com/stretchr/testify/require"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

var L = &Logger{}

type Logger struct{}

func (l *Logger) Info() *Logger { return l }
func (l *Logger) Str(key, value string) *Logger {
	fmt.Printf("%s: %s\n", key, value)
	return l
}
func (l *Logger) Msg(msg string) {
	fmt.Println(msg)
}
func (l *Logger) Err(err error) *Logger {
	fmt.Println(err)
	return l
}
func (l *Logger) Msgf(format string, args ...interface{}) {
	fmt.Printf(format+"\n", args...)
}

// Mock data structures
type Client struct {
	Cfg *Config
}

type Config struct {
	Network Network
}

type Network struct {
	Name string
}

var (
	validContent = `[[keys]]
private_key = '72a58427a0107d982a19f4f9feb3dd18ef95f36c7839b97b9064936e5a792eb0'
address = '0x15E28dC693cE24A289f8Cd7D424D54d2969EB7d7'
funds = '987316640856926500'`
)

var test_id = fmt.Sprintf("test-%s", uuid.New().String()[:5])

func TestOnePassFullFlow(t *testing.T) {
	client := &seth.Client{Cfg: &seth.Config{Network: &seth.Network{Name: test_id}}}
	vaultId := os.Getenv(seth.ONE_PASS_VAULT_ENV_VAR)

	err := seth.CreateIn1Pass(client, validContent, vaultId)
	require.NoError(t, err)

	defer func() {
		err = seth.DeleteFrom1Pass(client, vaultId)
		assert.NoError(t, err)
	}()

	exists, err := seth.ExistsIn1Pass(client, vaultId)
	require.NoError(t, err)
	require.True(t, exists)

	keyfile, err := seth.LoadFrom1Pass(client, vaultId)
	require.NoError(t, err)
	require.Equal(t, "72a58427a0107d982a19f4f9feb3dd18ef95f36c7839b97b9064936e5a792eb0", keyfile.Keys[0].PrivateKey)
	require.Equal(t, "0x15E28dC693cE24A289f8Cd7D424D54d2969EB7d7", keyfile.Keys[0].Address)
	require.Equal(t, "987316640856926500", keyfile.Keys[0].Funds)
}

func TestOnePassReplaceIn1Pass(t *testing.T) {
	client := &seth.Client{Cfg: &seth.Config{Network: &seth.Network{Name: test_id}}}
	vaultId := os.Getenv(seth.ONE_PASS_VAULT_ENV_VAR)

	newContent := `[[keys]]
private_key = '3c95fd73661aa090396723ef3ee0599fa509d8781a7c0bafe6d613c8664a03c7'
address = '0x105326Ff9D481d62c7458Ec1FCC9776F3809dfDB'
funds = '987316640856926501'`

	err := seth.CreateIn1Pass(client, validContent, vaultId)
	require.NoError(t, err)

	defer func() {
		err = seth.DeleteFrom1Pass(client, vaultId)
		assert.NoError(t, err)
	}()

	err = seth.ReplaceIn1Pass(client, newContent, vaultId)
	require.NoError(t, err)

	keyfile, err := seth.LoadFrom1Pass(client, vaultId)
	require.NoError(t, err)
	require.Equal(t, "3c95fd73661aa090396723ef3ee0599fa509d8781a7c0bafe6d613c8664a03c7", keyfile.Keys[0].PrivateKey)
	require.Equal(t, "0x105326Ff9D481d62c7458Ec1FCC9776F3809dfDB", keyfile.Keys[0].Address)
	require.Equal(t, "987316640856926501", keyfile.Keys[0].Funds)
}

func TestOnePassDoesNotExistsIn1Pass(t *testing.T) {
	client := &seth.Client{Cfg: &seth.Config{Network: &seth.Network{Name: "i-don-t-exist"}}}
	vaultId := os.Getenv(seth.ONE_PASS_VAULT_ENV_VAR)

	exists, err := seth.ExistsIn1Pass(client, vaultId)
	require.NoError(t, err)
	require.False(t, exists)
}

func TestOnePassLoadNonExistentFrom1Pass(t *testing.T) {
	client := &seth.Client{Cfg: &seth.Config{Network: &seth.Network{Name: "i-don-t-exist"}}}
	vaultId := os.Getenv(seth.ONE_PASS_VAULT_ENV_VAR)

	_, err := seth.LoadFrom1Pass(client, vaultId)
	require.Error(t, err)
}

func TestOnePassDeleteFrom1Pass(t *testing.T) {
	client := &seth.Client{Cfg: &seth.Config{Network: &seth.Network{Name: test_id}}}
	vaultId := os.Getenv(seth.ONE_PASS_VAULT_ENV_VAR)

	err := seth.CreateIn1Pass(client, validContent, vaultId)
	require.NoError(t, err)

	err = seth.DeleteFrom1Pass(client, vaultId)
	require.NoError(t, err)
}

func TestOnePassDeleteNonExistentFrom1Pass(t *testing.T) {
	client := &seth.Client{Cfg: &seth.Config{Network: &seth.Network{Name: "i-don-t-exist"}}}
	vaultId := os.Getenv(seth.ONE_PASS_VAULT_ENV_VAR)

	err := seth.DeleteFrom1Pass(client, vaultId)
	require.Error(t, err)
}
