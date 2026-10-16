#!/usr/bin/env bash
# Sets the image tag a service runs in one environment of rekreativko-infra; ArgoCD rolls out
# whatever lands on infra main. The tag lives in k8s/overlays/<env>/<overlay>/kustomization.yaml
# under images: (k8s/base carries no tags, so environments never share one).
#
# usage: bump-infra-image.sh <environment> <service> <tag>
# env:   GH_TOKEN         token with contents:write (and pull-requests:write for MODE=pr)
#                         on rekreativko-infra
#        MODE             "push" (default): commit straight to infra main - the dev auto-deploy
#                         "pr": open a PR instead, so the release happens when it is merged;
#                         run from a full-history rekreativko-api checkout so the PR can list
#                         the commits being released
#        SKIP_IF_MISSING  "true" to exit 0 when the environment has no overlay for the service
#                         (a service that is built before it is deployed anywhere)
set -euo pipefail

ENV="$1"
SVC="$2"
TAG="$3"
MODE="${MODE:-push}"

IMAGE_NAME="ghcr.io/rasparac/${SVC}"
API_DIR="$PWD"

# activity-cron is a second image deployed from the activity overlay (and built from activity/)
OVERLAY="$SVC"
if [ "$SVC" = "activity-cron" ]; then
  OVERLAY="activity"
fi
FILE="k8s/overlays/${ENV}/${OVERLAY}/kustomization.yaml"

git clone -q "https://x-access-token:${GH_TOKEN}@github.com/rasparac/rekreativko-infra.git" infra-repo
cd infra-repo
git config user.name "rekreativko-ci"
git config user.email "ci@rekreativko.local"

if [ ! -f "$FILE" ]; then
  if [ "${SKIP_IF_MISSING:-}" = "true" ]; then
    echo "::notice::no ${FILE} in rekreativko-infra, ${SVC} not deployed to ${ENV}"
    exit 0
  fi
  echo "::error::no ${FILE} in rekreativko-infra, ${SVC} is not deployed to ${ENV}"
  exit 1
fi

# yq would silently change nothing if the overlay doesn't list the image
if [ "$(IMAGE_NAME="$IMAGE_NAME" yq '[.images[] | select(.name == strenv(IMAGE_NAME))] | length' "$FILE")" != "1" ]; then
  echo "::error::${FILE} has no images entry for ${IMAGE_NAME}"
  exit 1
fi

set_tag() {
  IMAGE_NAME="$IMAGE_NAME" TAG="$TAG" \
    yq -i '(.images[] | select(.name == strenv(IMAGE_NAME))).newTag = strenv(TAG)' "$FILE"
  git add "$FILE"
}

# <date>-<sha7> tags carry the commit they were built from; prints nothing for any other tag
tag_commit() {
  local sha="${1##*-}"
  if [[ "$sha" =~ ^[0-9a-f]{7,40}$ ]] && git -C "$API_DIR" cat-file -e "${sha}^{commit}" 2>/dev/null; then
    echo "$sha"
  fi
}

if [ "$MODE" = "pr" ]; then
  CURRENT=$(IMAGE_NAME="$IMAGE_NAME" yq '.images[] | select(.name == strenv(IMAGE_NAME)) | .newTag' "$FILE")
  set_tag
  if git diff --cached --quiet; then
    echo "::notice::${SVC} already runs ${TAG} in ${ENV}, nothing to release"
    exit 0
  fi

  BRANCH="release/${ENV}/${SVC}/${TAG}"
  if git ls-remote --exit-code --heads origin "$BRANCH" > /dev/null; then
    echo "::error::branch ${BRANCH} already exists in rekreativko-infra - a release PR for it is probably open"
    exit 1
  fi

  TITLE="release(${ENV}/${SVC}): ${TAG}"
  git switch -q -c "$BRANCH"
  git commit -q -m "$TITLE"
  git push -q origin "$BRANCH"

  # The code the release brings in (or, on a rollback, takes out), limited to what the image is built from
  FROM=$(tag_commit "$CURRENT")
  TO=$(tag_commit "$TAG")
  SRC_PATHS=("${OVERLAY}/" shared/ go.mod go.sum build/Dockerfile)
  BODY=$(mktemp)
  {
    echo "Releases \`${IMAGE_NAME}\` to **${ENV}**: \`${CURRENT}\` → \`${TAG}\`."
    echo
    echo "Merging deploys it (ArgoCD syncs infra main); closing the PR cancels the release."
    echo
    if [ -n "$FROM" ] && [ -n "$TO" ]; then
      if git -C "$API_DIR" merge-base --is-ancestor "$TO" "$FROM"; then
        echo "### ⚠️ Rollback - these commits leave ${ENV}"
        RANGE="${TO}..${FROM}"
      else
        echo "### Commits"
        RANGE="${FROM}..${TO}"
      fi
      echo
      LOG=$(git -C "$API_DIR" log --format='- %h %s (%an)' "$RANGE" -- "${SRC_PATHS[@]}")
      echo "${LOG:-_No changes to ${OVERLAY}/, shared/ or the build - the image is a rebuild of the same code._}"
      echo
      echo "Full diff: https://github.com/rasparac/rekreativko-api/compare/${FROM}...${TO}"
    else
      echo "_Can't list commits: \`${CURRENT}\` or \`${TAG}\` doesn't name a commit in rekreativko-api._"
    fi
    echo
    echo "Opened by the [Release workflow](${GITHUB_SERVER_URL:-https://github.com}/${GITHUB_REPOSITORY:-rasparac/rekreativko-api}/actions/runs/${GITHUB_RUN_ID:-}) for ${GITHUB_ACTOR:-unknown}."
  } > "$BODY"

  PR_URL=$(gh pr create --repo rasparac/rekreativko-infra --base main --head "$BRANCH" --title "$TITLE" --body-file "$BODY")
  echo "::notice::release PR opened: ${PR_URL}"
  if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
    { echo "Release PR: ${PR_URL}"; echo; cat "$BODY"; } >> "$GITHUB_STEP_SUMMARY"
  fi
  exit 0
fi

PUSHED=false
for i in 1 2 3 4 5; do
  set_tag

  if git diff --cached --quiet; then
    echo "${SVC} already runs ${TAG} in ${ENV}, nothing to commit"
    PUSHED=true
    break
  fi

  git commit -q -m "chore(${ENV}/${SVC}): set image to ${TAG}"

  if git push -q origin HEAD:main; then
    echo "${ENV}: ${SVC} -> ${TAG}"
    PUSHED=true
    break
  fi

  echo "push rejected (attempt $i), rebasing onto latest main and retrying..."
  git fetch -q origin main
  git reset -q --hard origin/main
  sleep $((RANDOM % 5 + 1))
done

if [ "$PUSHED" != "true" ]; then
  echo "::error::failed to push ${ENV} image bump for ${SVC} after 5 attempts"
  exit 1
fi
