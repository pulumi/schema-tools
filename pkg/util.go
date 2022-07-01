package pkg

import (
	"encoding/json"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"io/ioutil"
	"net/http"
)

func DownloadSchema(schemaUrl string) (schema.PackageSpec, error) {
	resp, err := http.Get(schemaUrl)
	if err != nil {
		return schema.PackageSpec{}, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return schema.PackageSpec{}, err
	}

	var sch schema.PackageSpec
	if err = json.Unmarshal(body, &sch); err != nil {
		return schema.PackageSpec{}, nil
	}

	return sch, nil
}

func LoadLocalPackageSpec(filePath string) (schema.PackageSpec, error) {
	body, err := ioutil.ReadFile(filePath)
	if err != nil {
		return schema.PackageSpec{}, err
	}

	var sch schema.PackageSpec
	if err = json.Unmarshal(body, &sch); err != nil {
		return schema.PackageSpec{}, err
	}

	return sch, nil
}
