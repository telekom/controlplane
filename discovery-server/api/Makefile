# Copyright 2025 Deutsche Telekom IT GmbH
#
# SPDX-License-Identifier: Apache-2.0

.PHONY: generate

generate:
	openapi-merge-cli --config openapi-merge.json
	sed -i "s|common\.yaml#/\(.*\)|'#/\1'|g" uber-openapi.yaml
	cd .. && go generate ./...
