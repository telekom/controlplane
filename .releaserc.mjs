// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// @ts-check

/** @type {import('semantic-release').GlobalConfig} */
export default {
    preset: 'angular',
    tagFormat: 'v${version}',
    repositoryUrl: 'https://github.com/telekom/controlplane.git',
    branches: [
        'master',
        'main',
        'next',
        'next-major',
    ],
    plugins: [
        '@semantic-release/commit-analyzer',
        '@semantic-release/release-notes-generator',
        '@semantic-release/changelog',
        ['@semantic-release/exec', {
            prepareCmd: `
                bash ./.github/scripts/update_install.sh "\${nextRelease.gitTag}"
                bash ./.github/scripts/update_chart_version.sh common-server/helm "\${nextRelease.gitTag}"
            `,
            publishCmd: `cat > /tmp/release-notes.md <<'EOF'
\${nextRelease.notes}
EOF`,
        }],
        ['@semantic-release/git', {
            assets: [
                'CHANGELOG.md',
                'install/overlays/default/kustomization.yaml',
                'common-server/helm/Chart.yaml',
            ],
            // @semantic-release/git's default commit message includes
            // "[skip ci]". release-publish.yaml is triggered by the `v*`
            // tag push that immediately follows this commit, and GitHub
            // applies skip-ci directives to that tag's push event too
            // (both point at the same commit) - a "[skip ci]" message here
            // would silently suppress release-publish.yaml. Omit it.
            message: 'chore(release): ${nextRelease.gitTag}\n\n${nextRelease.notes}',
        }],
    ],
};
