#!/bin/bash -ex


HUGO_VERSION="v0.122.0"

case "$1" in
	"")
      go run "github.com/gohugoio/hugo@${HUGO_VERSION}" server
	  ;;
	"--")
      go run "github.com/gohugoio/hugo@${HUGO_VERSION}" ${@:2}
	  ;;
	"update")
      go run "github.com/gohugoio/hugo@${HUGO_VERSION}"

      SCRATCH_DIR="$( mktemp -d )"
      TARGET_DIR="${SCRATCH_DIR}/site"
      git clone git@github.com:ericchiang/ericchiang.github.io.git "${TARGET_DIR}"

	  rsync -a public/ "${TARGET_DIR}"

	  cd "${TARGET_DIR}"
	  git status

	  while true; do
		  echo -n "Deploy? [yes/no/diff]: "
		  read COMMAND
		  DONE=""
		  case "$COMMAND" in
			  "yes")
				  git add .
				  git commit -m "Update $(date)"
				  git push origin master
				  DONE="true"
				  ;;
			  "no")
				  echo "Not deploying"
				  DONE="true"
				  ;;
			  "diff")
				  git diff
				  ;;
			  "*")
				  echo "Unrecognized command ${COMMAND}"
				  ;;
		  esac
		  if [[ "$DONE" != "" ]]; then
			  break
		  fi
	  done
      rm -rf "${SCRATCH_DIR}"
	  ;;
	*)
	  echo "Unrecognized command ${1}"
	  exit 1
	  ;;
esac
