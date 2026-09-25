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
    // Deliberately does NOT use @semantic-release/changelog or
    // @semantic-release/release-notes-generator: the changelog itself is no
    // longer a committed CHANGELOG.md file. GoReleaser generates it from
    // commits since the previous tag and publishes it as the GitHub Release
    // body (see .goreleaser.yaml's `changelog:`/`release:` config) instead.
    //
    // Does use @semantic-release/git, scoped to only the two self-referential
    // version-bump files below (not CHANGELOG.md). This is deliberate despite
    // semantic-release's own general advice to avoid the git plugin when
    // possible:
    //   - install/overlays/default/kustomization.yaml's `ref=` value points
    //     at the very tag being cut, so the *tagged commit's own tree* must
    //     already contain the bumped value - not just a later commit on
    //     `main`.
    //   - semantic-release *core* (not the git plugin) creates and pushes
    //     the release tag itself, pointing at whatever commit is HEAD after
    //     the `prepare` step - see
    //     https://github.com/semantic-release/semantic-release/discussions/3800.
    //     If that commit is never pushed to the release branch, it becomes
    //     unreachable from `main`. semantic-release's *next* run determines
    //     the "last release" by looking at tags reachable from the current
    //     branch, so an unreachable tag is invisible to it - it would
    //     recompute from the previous (older) release and could attempt to
    //     recreate the same version/tag again. Pushing this commit to the
    //     branch (what @semantic-release/git does) is what keeps the tag
    //     reachable and next-version calculation correct.
    //   - The commit message below intentionally does NOT include `[skip
    //     ci]` (the plugin's default message does): GitHub applies skip
    //     directives to the `push` event of the *tag* too, since both the
    //     branch commit and the tag point at the same commit - a skip
    //     marker here would silently suppress release-publish.yaml.
    //   - Trade-off accepted: this bot commit bypasses required PR review
    //     on the protected branch (the bot has the necessary bypass rights),
    //     and - since it uses the same GitHub App token as the tag push -
    //     also re-triggers ci.yaml's ordinary push-to-main CI run. Both are
    //     pre-existing, previously-accepted behaviors of this release
    //     process, not new costs introduced here.
    plugins: [
        '@semantic-release/commit-analyzer',
        ['@semantic-release/exec', {
            prepareCmd: `
                bash ./.github/scripts/update_install.sh "\${nextRelease.gitTag}"
                bash ./.github/scripts/update_chart_version.sh common-server/helm "\${nextRelease.gitTag}"
            `,
        }],
        ['@semantic-release/git', {
            assets: [
                'install/overlays/default/kustomization.yaml',
                'common-server/helm/Chart.yaml',
            ],
            message: 'chore(release): ${nextRelease.gitTag}',
        }],
    ],
};
