# schema-tools

Tools to analyze Pulumi schemas.

## Building

```shell
go install
go build
```

## Usage

```shell
Available Commands:
  compare     Compare two versions of a Pulumi schema
  completion  Generate the autocompletion script for the specified shell
  help        Help about any command
  stats       Get the stats of a current schema
  version     Print the version number of schema-tools
```

## Resource Stats

### Latest commit on 'master'

``` shell
$ schema-tools stats --provider azure-native
Provider: azure-native
Total resources: 1056
Unique resources: 1056
Total properties: 13018
```

### Specific tag or commit

```shell
schema-tools $./schema-tools stats -p aws -t v5.41.0
Provider: aws
{
  "Functions": {
    "TotalFunctions": 506,
    "TotalDescriptionBytes": 1596936,
    "TotalInputPropertyDescriptionBytes": 98569,
    "InputPropertiesMissingDescriptions": 46,
    "TotalOutputPropertyDescriptionBytes": 0,
    "OutputPropertiesMissingDescriptions": 0
  },
  "Resources": {
    "TotalResources": 1210,
    "TotalDescriptionBytes": 10153011,
    "TotalInputProperties": 12703,
    "InputPropertiesMissingDescriptions": 507,
    "TotalOutputProperties": 13722,
    "OutputPropertiesMissingDescriptions": 709
  }
}
```

### Specific tag or commit with details

```shell
schema-tools $./schema-tools stats -p docker -t v4.3.1 -d
Provider: docker
{
  "Functions": {
    "TotalFunctions": 5,
    "TotalDescriptionBytes": 10325,
    "TotalInputPropertyDescriptionBytes": 601,
    "InputPropertiesMissingDescriptions": 8,
    "TotalOutputPropertyDescriptionBytes": 0,
    "OutputPropertiesMissingDescriptions": 0
  },
  "Resources": {
    "TotalResources": 11,
    "TotalDescriptionBytes": 35810,
    "TotalInputProperties": 258,
    "InputPropertiesMissingDescriptions": 56,
    "TotalOutputProperties": 212,
    "OutputPropertiesMissingDescriptions": 56
  }
}

### All Resources:

docker:index/container:Container
docker:index/image:Image
docker:index/network:Network
docker:index/plugin:Plugin
docker:index/registryImage:RegistryImage
docker:index/remoteImage:RemoteImage
docker:index/secret:Secret
docker:index/service:Service
docker:index/serviceConfig:ServiceConfig
docker:index/tag:Tag
docker:index/volume:Volume

### All Functions:

docker:index/getLogs:getLogs
docker:index/getNetwork:getNetwork
docker:index/getPlugin:getPlugin
docker:index/getRegistryImage:getRegistryImage
docker:index/getRemoteImage:getRemoteImage
```

## Schema Comparison

To review potential breaking changes between master and a newer commit from a PR:

```shell
$ schema-tools compare -p aws -o master -n 4379b20d1aab018bac69c6d86c4219b08f8d3ec4
Found 1 breaking change:
Function "aws:s3/getBucketObject:getBucketObject" missing input "bucketKeyEnabled"
```

To review historical changes between two commits or tags like v3 and v4:

```shell
(base) schema-tools $schema-tools compare -p docker -o v3.0.0 -n v4.0.0
Found 3 breaking changes:
Function "docker:index/getNetwork:getNetwork" missing input "id"
Type "docker:index/ServiceTaskSpecResourcesLimits:ServiceTaskSpecResourcesLimits" missing property "genericResources"
Type "docker:index/ServiceTaskSpecResourcesLimitsGenericResources:ServiceTaskSpecResourcesLimitsGenericResources" missing

#### New resources:

- `index/image.Image`
- `index/tag.Tag`

#### New functions:

- `index/getLogs.getLogs`
- `index/getRemoteImage.getRemoteImage`
```
