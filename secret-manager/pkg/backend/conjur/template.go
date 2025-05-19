// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package conjur

const startTag = "{{"
const endTag = "}}"

const EnvironmentPolicyTemplate = `
- !policy
  id: {{Environment}}
  body:
  - !variable zones
`

const TeamPolicyTemplate = `
- !policy
  id: {{TeamId}}
  body:
  - !variable clientSecret
  - !variable teamToken
`

const ApplicationPolicyTemplate = `
- !policy
  id: {{AppId}}
  body:
  - !variable clientSecret
  - !variable externalSecrets
`

const DeletePolicyTemplate = `
- !delete
  record: !policy {{PolicyPath}}
`
