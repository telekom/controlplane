<!--
Copyright 2025 Deutsche Telekom IT GmbH

SPDX-License-Identifier: Apache-2.0
-->

# Route-Testing Tool

This tool can be used to test if a Route is correctly configured. Its basically an automated version of `curl`.



## Usage


Install the tool with:
```bash
go build -o bin/rt main.go
install -m 0755 bin/rt /usr/local/bin/rt
# or go run main.go --help
rt --help
```

Run the tool with:
```bash
rt --app rover-sample-consumer --basepath /eni/foo/v2 --team sample-team --env poc | jq 
```