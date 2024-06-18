package seth

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/pkg/errors"
)

// CreateIn1Pass creates a new keyfile in 1Password. If a keyfile with the same name already exists we will return an error.
// Keyfile will be added as a file attachment to a Secure Note in the specified vault.
func CreateIn1Pass(c *Client, content string, vaultId string) error {
	absolutePath, keyName, err := validateInputsAndGetKeyNameAndPath(c, content, vaultId)
	if err != nil {
		return err
	}
	L.Info().Str("Item name", keyName).Msg("Creating keyfile in 1Password")

	if exists, _ := ExistsIn1Pass(c, vaultId); exists {
		err := fmt.Errorf("keyfile with the same name '%s' already exists in 1Password", keyName)
		L.Error().Err(err).Msg("Keyfile creation failed")
		return err
	}

	uploadCmd := exec.Command("op", "item", "create", "--vault", vaultId, "--category", "Secure Note", "--title", keyName, fmt.Sprintf("keyfile[file]=%s", absolutePath))
	output, err := uploadCmd.CombinedOutput()
	if err != nil {
		L.Err(err).Msgf("failed to upload keyfile to 1Password:\n%s", string(output))
		if len(output) > 0 {
			return errors.Wrapf(errors.New(string(output)), "failed to create keyfile in 1Password")
		}
		return errors.Wrapf(err, "failed to create keyfile in 1Password")
	}
	L.Info().Str("Item name", keyName).Msg("Keyfile created in 1Password")

	return nil
}

// ReplaceIn1Pass replaces the keyfile in 1Password. If a keyfile with the same name does not exist it will return an error.
func ReplaceIn1Pass(c *Client, content string, vaultId string) error {
	absolutePath, keyName, err := validateInputsAndGetKeyNameAndPath(c, content, vaultId)
	if err != nil {
		return err
	}
	L.Info().Str("Item name", keyName).Msg("Replacing keyfile in 1Password")

	uploadCmd := exec.Command("op", "item", "edit", "--vault", vaultId, keyName, fmt.Sprintf("keyfile[file]=%s", absolutePath))
	output, err := uploadCmd.Output()
	if err != nil {
		L.Err(err).Msgf("failed to replace keyfile in 1Password:\n%s", string(output))
		return errors.Wrapf(err, "failed to reaplce keyfile in 1Password")
	}
	L.Info().Str("Item name", keyName).Msg("Keyfile replaced in 1Password")

	return nil
}

func validateInputsAndGetKeyNameAndPath(c *Client, content string, vaultId string) (keyfilePath string, keyName string, err error) {
	if installed := isOpInstalled(); !installed {
		return "", "", errors.New("1Password CLI is not installed")
	}

	if vaultId == "" {
		return "", "", errors.New("vault name or id is required")
	}

	if content == "" {
		return "", "", errors.New("no content to save, you passed an empty string")
	}

	var tmpFile *os.File
	tmpFile, err = os.CreateTemp("", "keyfile.toml")
	if err != nil {
		return "", "", errors.Wrapf(err, "failed to create temporary file")
	}

	_, err = tmpFile.WriteString(content)
	if err != nil {
		return "", "", errors.Wrapf(err, "failed to write content to temporary file")
	}

	keyfilePath, err = filepath.Abs(tmpFile.Name())
	if err != nil {
		return "", "", errors.Wrapf(err, "failed to get absolute path of temporary keyfile file")
	}

	keyName = generate1PassKeyName(c.Cfg)

	return
}

// ExistsIn1Pass checks if a keyfile exists in the specified vault.
func ExistsIn1Pass(c *Client, vaultId string) (bool, error) {
	if installed := isOpInstalled(); !installed {
		return false, errors.New("1Password CLI is not installed")
	}

	if vaultId == "" {
		return false, errors.New("vault name or id is required")
	}

	keyName := generate1PassKeyName(c.Cfg)

	checkCmd := exec.Command("op", "item", "get", keyName, "--vault", vaultId)
	_, err := checkCmd.Output()

	return err == nil, nil
}

// LoadFrom1Pass loads a keyfile from 1Password. If the keyfile does not exist it will return an error.
func LoadFrom1Pass(c *Client, vaultId string) (KeyFile, error) {
	if installed := isOpInstalled(); !installed {
		return KeyFile{}, errors.New("1Password CLI is not installed")
	}

	if vaultId == "" {
		return KeyFile{}, errors.New("vault name or id is required")
	}

	keyName := generate1PassKeyName(c.Cfg)
	L.Info().Str("Item name", keyName).Msg("Loading keyfile from 1Password")

	downloadCmd := exec.Command("op", "read", fmt.Sprintf("op://%s/%s/keyfile", vaultId, keyName))
	output, err := downloadCmd.Output()
	if err != nil {
		L.Err(err).Msgf("failed to load keyfile from 1Password:\n%s", string(output))
		return KeyFile{}, errors.Wrapf(err, "failed to load keyfile from 1Password")
	}

	keyfile := KeyFile{}
	err = toml.Unmarshal(output, &keyfile)
	if err != nil {
		return KeyFile{}, errors.Wrapf(err, "failed to unmarshal keyfile")
	}
	L.Info().Str("Item name", keyName).Msg("Keyfile loaded from 1Password")

	return keyfile, nil
}

func DeleteFrom1Pass(c *Client, vaultId string) error {
	if installed := isOpInstalled(); !installed {
		return errors.New("1Password CLI is not installed")
	}

	if vaultId == "" {
		return errors.New("vault name or id is required")
	}

	keyName := generate1PassKeyName(c.Cfg)
	L.Info().Str("Item name", keyName).Msg("Deleting keyfile from 1Password")

	checkCmd := exec.Command("op", "item", "delete", keyName, "--vault", vaultId)
	_, err := checkCmd.Output()
	L.Info().Str("Item name", keyName).Msg("Keyfile deleted from 1Password")

	return err
}

func isOpInstalled() bool {
	opExistsCmd := exec.Command("op", "--version")
	_, err := opExistsCmd.Output()
	return err == nil
}

func generate1PassKeyName(cfg *Config) string {
	return strings.ToUpper(cfg.Network.Name) + "_KEYFILE"
}
