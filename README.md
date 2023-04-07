# schema-tools

Tools to analyze Pulumi schemas.

## Building

```
go install
go build
```
## Usage
```
Available Commands:
  compare     Compare two versions of a Pulumi schema
  completion  Generate the autocompletion script for the specified shell
  help        Help about any command
  stats       Get the stats of a current schema
  version     Print the version number of schema-tools
```
## Resource Stats

```
$ schema-tools stats --provider azure-native
Provider: azure-native
Total resources: 1056
Unique resources: 1056
Total properties: 13018
```

## Schema Comparison

To review potential breaking changes between master and a newer commit from a PR:

```
$ schema-tools compare -p aws -o master -n 4379b20d1aab018bac69c6d86c4219b08f8d3ec4
Found 1 breaking change:
Function "aws:s3/getBucketObject:getBucketObject" missing input "bucketKeyEnabled"
```

To review historical changes between two commits or tags like v3 and v4:
```
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
