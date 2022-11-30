package update

import (
	"encoding/json"
	"io"
	"os"
)

//const npmrc = "//registry.npmjs.org/:_authToken=${NPM_TOKEN}\n"

func init() {
	Register("package.json", packageJson)
}

func packageJson(newVersion string, file *os.File) error {
	var data map[string]json.RawMessage
	if err := json.NewDecoder(file).Decode(&data); err != nil {
		return err
	}
	data["version"] = json.RawMessage("\"" + newVersion + "\"")
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := file.Truncate(0); err != nil {
		return err
	}
	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		return err
	}
	//return ioutil.WriteFile(path.Join(path.Dir(file.Name()), ".npmrc"), []byte(npmrc), 0644)
	return nil
}
