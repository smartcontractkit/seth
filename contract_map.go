package seth

import (
	"io"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/pelletier/go-toml/v2"
)

func SaveDeployedContract(filename, contractName, address string) error {
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)

	if err != nil {
		return err
	}
	defer file.Close()

	v := map[string]string{
		address: contractName,
	}

	marhalled, err := toml.Marshal(v)
	if err != nil {
		return err
	}

	_, err = file.WriteString(string(marhalled))
	return err
}

func LoadDeployedContracts(filename string) (map[string]string, error) {
	tomlFile, err := os.Open(filename)
	if err != nil {
		return map[string]string{}, nil
	}
	defer tomlFile.Close()

	b, _ := io.ReadAll(tomlFile)
	rawContracts := map[common.Address]string{}
	err = toml.Unmarshal(b, &rawContracts)
	if err != nil {
		return map[string]string{}, err
	}

	contracts := map[string]string{}
	for k, v := range rawContracts {
		contracts[k.Hex()] = v
	}

	return contracts, nil
}
