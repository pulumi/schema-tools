package pkg

import (
	"encoding/json"
	"fmt"
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

	if resp.StatusCode == 404 {
		err := fmt.Errorf("received a 404 response attempting to download schema from '%s'",
			schemaUrl)
		return schema.PackageSpec{}, err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return schema.PackageSpec{}, err
	}

	var sch schema.PackageSpec
	if err = json.Unmarshal(body, &sch); err != nil {
		return schema.PackageSpec{}, err
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
