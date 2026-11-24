#!/usr/bin/env bash
# Tag a release: make release version=X.Y.Z summary="<one sentence>" [confirm=yes]
#
# Copy to scripts/release.sh when the repo's Git profile sets `tags: vX.Y.Z` (makefile skill, release.mk).
# It only makes the tag. The pushed tag starts CI, which builds the image and creates the release page
# (git-flow, references/release-tags.md).
#
# Without confirm=yes it only prints the tag message (a preview). With confirm=yes it creates the annotated
# tag vX.Y.Z on HEAD and pushes it. Refuses unless HEAD is a clean main in sync with origin and the tag is new:
# tags are never moved.
set -euo pipefail

version="${1:-}"
summary="${2:-}"
confirm="${3:-}"

usage() {
    echo 'usage: make release version=X.Y.Z summary="<what this release delivers>" [confirm=yes]' >&2
    exit 1
}
die() {
    echo "release: $*" >&2
    exit 1
}

[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || usage
[[ -n "$summary" ]] || usage
tag="v$version"

grep -Eqs '^tags:[[:space:]]*vX\.Y\.Z' CLAUDE.md AGENTS.md \
    || die "the Git profile (CLAUDE.md or AGENTS.md) does not set 'tags: vX.Y.Z'"
[[ "$(git branch --show-current)" == main ]] || die "release from main only"
[[ -z "$(git status --porcelain)" ]] || die "working tree not clean"
git fetch --quiet --tags origin main
[[ "$(git rev-parse HEAD)" == "$(git rev-parse origin/main)" ]] || die "main is not in sync with origin/main"
if git rev-parse -q --verify "refs/tags/$tag" >/dev/null \
    || [[ -n "$(git ls-remote --tags origin "refs/tags/$tag")" ]]; then
    die "$tag already exists; tags are never moved, pick the next version"
fi

previous="$(git describe --tags --abbrev=0 --match 'v[0-9]*' 2>/dev/null || true)"
range="${previous:+$previous..}HEAD"

# One line per commit, grouped like GoReleaser's changelog (Features / Fixes / Other), so `release: page`
# and `release: goreleaser` notes read the same. docs: and Renovate's chore(deps): are left out.
features="" fixes="" other=""
while IFS= read -r subject; do
    case "$subject" in
        feat:* | feat\(*\):* | feat!:* | feat\(*\)!:*) features+="- $subject"$'\n' ;;
        fix:* | fix\(*\):* | fix!:* | fix\(*\)!:*) fixes+="- $subject"$'\n' ;;
        docs:* | docs\(*\):* | "chore(deps):"*) ;;
        *) other+="- $subject"$'\n' ;;
    esac
done < <(git log --no-merges --reverse --format='%s' "$range")

[[ -n "$features$fixes$other" ]] || die "no changes since ${previous:-the first commit} to release"

message="$summary"$'\n'
[[ -z "$features" ]] || message+=$'\n'"Features:"$'\n\n'"$features"
[[ -z "$fixes" ]] || message+=$'\n'"Fixes:"$'\n\n'"$fixes"
[[ -z "$other" ]] || message+=$'\n'"Other:"$'\n\n'"$other"

echo "$tag (changes since ${previous:-the first commit}):"
echo
echo "$message"

if [[ "$confirm" != yes ]]; then
    echo "preview only: rerun with confirm=yes to tag and push $tag (the push starts the release CI)"
    exit 0
fi

git tag -a "$tag" -F - <<<"$message"
git push origin "$tag"
echo "pushed $tag: watch the tag's CI runs and report the result (git-flow)"
