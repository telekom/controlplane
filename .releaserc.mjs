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
    // Deliberately does NOT use @semantic-release/git or
    // @semantic-release/changelog:
    //   - The changelog itself is no longer a committed CHANGELOG.md file;
    //     GoReleaser generates it from commits since the previous tag and
    //     publishes it as the GitHub Release body (see .goreleaser.yaml's
    //     `changelog:`/`release:` config) - semantic-release's own release
    //     notes plugin is unnecessary for the same reason.
    //   - semantic-release *core* (not the git plugin) already creates and
    //     pushes the release tag on its own - see
    //     https://github.com/semantic-release/semantic-release/discussions/3800.
    //     The git plugin's only remaining job would be committing/pushing
    //     version-bump files to the branch, which carries real downsides
    //     (bypasses required PR review on protected branches, can race with
    //     concurrent pushes, and - since this workflow deliberately uses a
    //     GitHub App token so the *tag* push triggers release-publish.yaml -
    //     would also spuriously re-trigger ci.yaml's push-to-main CI run).
    //   - The `@semantic-release/exec` prepare hook below still needs
    //     install/overlays/default/kustomization.yaml and
    //     common-server/helm/Chart.yaml to carry the new version, since
    //     they're self-referential (the kustomize overlay's `ref=` value
    //     points at the very tag being cut). It commits them locally (see
    //     .github/scripts/commit_release_files.sh) so they become part of
    //     the commit that core then tags - but does not push to `main`.
    //     This means `main`'s copies of these two files reflect the last
    //     release only for as long as no further commits land on `main`;
    //     consumers should always pin to a release tag, never to `main`,
    //     which is the intended usage already.
    plugins: [
        '@semantic-release/commit-analyzer',
        ['@semantic-release/exec', {
            prepareCmd: `
                bash ./.github/scripts/update_install.sh "\${nextRelease.gitTag}"
                bash ./.github/scripts/update_chart_version.sh common-server/helm "\${nextRelease.gitTag}"
                bash ./.github/scripts/commit_release_files.sh "\${nextRelease.gitTag}"
            `,
        }],
    ],
};
