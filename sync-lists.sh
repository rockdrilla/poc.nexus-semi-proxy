#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# (c) 2023, Konstantin Demin

set -ef

me="${0##*/}"
usage() {
	cat >&2 <<-EOF
	# usage: ${me} <distribution> <suite>
	EOF
	exit "${1:-0}"
}
[ $# != 0 ] || usage

: "${NEXUS_URI:?}" "${NEXUS_AUTH:?}"

# NB: prefer HTTPS mirrors
: "${MIRROR_URI_DEBIAN:=https://deb.debian.org/debian}"
# : "${MIRROR_URI_UBUNTU:=http://archive.ubuntu.com/ubuntu}"
: "${MIRROR_URI_UBUNTU:=https://ftp.uni-stuttgart.de/ubuntu}"
: "${NEXUS_REPO_NAME_FMT:=raw-lists-%s-%s}"
: "${COMP:=main}"
# 'all' arch is implicit
: "${ARCH:=amd64}"
: "${CONTENTS:=false}"
: "${SOURCES:=false}"
: "${DEP11:=false}"
: "${I18N:=en}"

arg_ok=
while : ; do
	[ -n "$1" ] || break
	[ -n "$2" ] || break
	arg_ok=1
break ; done
[ -n "${arg_ok}" ] || usage 1

DISTRO=$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')
case "${DISTRO}" in
debian ) MIRROR_URI=${MIRROR_URI_DEBIAN} ;;
ubuntu ) MIRROR_URI=${MIRROR_URI_UBUNTU} ;;
* )
	env printf "unsupported distro: %q\n" "${DISTRO}" >&2
	exit 1
;;
esac

SUITE=$(printf '%s' "$2" | tr '[:upper:]' '[:lower:]')
NEXUS_REPO=$(printf "${NEXUS_REPO_NAME_FMT}" "${DISTRO}" "${SUITE}")

_bool_norm() {
	case "$1" in
	1 | [Tt] | [Tt][Rr][Uu][Ee] )
		echo 1
	;;
	* ) echo 0 ;;
	esac
}

_list_norm() {
	printf '%s' "$1" | tr -s '[:space:]' '\0' | sort -zuV | paste -zsd' ' | tr -d '\0'
}

SOURCES=$(_bool_norm "${SOURCES}")
CONTENTS=$(_bool_norm "${CONTENTS}")
DEP11=$(_bool_norm "${DEP11}")

case "${I18N}" in
0 | [Ff][Aa][Ll][Ss][Ee] )
	I18N=
;;
[Ee][Nn] ) ;;
* )
	I18N="en ${I18N}"
	I18N=$(_list_norm "${I18N}")
;;
esac

_have_cmd() {
	command -V "$1" >/dev/null || {
		echo "missing command: $1"
		exit 1
	}
}

_have_cmd curl
## TODO: verify signatures
# _have_cmd gpg

PATH="$(readlink -e "$(dirname "$0")"):${PATH}"
export PATH

_curl="curl ${CURL_OPT} -sSL"
_curl() { ${_curl} "$@" ; }

_curl_test_uri_presense() {
	{
	echo 404
	_curl -I -o /dev/null -D - "$1" | mawk '/^HTTP\/[.0-9]+ [0-9]+( .*)?$/{print $2}' || :
	} \
	| tail -n 1
}

_curl_test_uri() { _curl_test_uri_presense "$1" | grep -Fxq '200' ; }

_test_uri() {
	_curl_test_uri "${MIRROR_URI}/dists/${SUITE}/$1"
}

_fetch_uri() {
	_curl --create-dirs -o "./$1" "${MIRROR_URI}/dists/${SUITE}/$1"
}

_put_file() {
	__dir="${1%/*}"
	[ "${__dir}" != "$1" ] || __dir=

	${__dir:+ env -C "${__dir}" } \
	${_curl} -D - -o /dev/null \
	  -X POST \
	  -F "raw.directory=/${__dir:+${__dir}/}" \
	  -F "raw.asset1.filename=${1##*/}" \
	  -F "raw.asset1=@${1##*/}" \
	  -u "${NEXUS_AUTH}" \
	"${NEXUS_URI}/service/rest/v1/components?repository=${NEXUS_REPO}"

	unset __dir
}

w=$(mktemp -d) ; : "${w:?}"
cd "$w"

_cleanup() { cd / ; rm -rf "$w" ; }

(

if _fetch_uri Release ; then
	_fetch_uri Release.gpg
fi
_fetch_uri InRelease

## TODO: verify signatures

if [ "${COMP}" = ':all' ] || [ -z "${COMP}" ] ; then
	COMP=$(deb822-get-field.sh Components ./Release)
fi
COMP=$(_list_norm "${COMP}")

if [ "${ARCH}" = ':all' ] || [ -z "${ARCH}" ] ; then
	ARCH=$(deb822-get-field.sh Architectures ./Release)
else
	ARCH="all ${ARCH}"
fi
ARCH=$(_list_norm "${ARCH}")

for f in MD5Sum SHA1 SHA256 ; do
	deb822-get-field.sh "$f" ./Release
done | mawk '{print $3}' | sort -uV > .list

if ! [ -s .list ] ; then
	echo "unable to fetch file list for ${SUITE} suite in ${MIRROR_URI}"
	exit 1
fi

# disable safety due to "grep"
set +e

for comp in ${COMP} ; do
	for arch in ${ARCH} ; do
		grep -E "^${comp}/binary-${arch}/(Packages|Release)(\$|\\.)" .list

		if [ "${CONTENTS}" = 1 ] ; then
			grep -E "^${comp}/Contents-${arch}(\$|\\.)" .list
		fi

		if [ "${DEP11}" = 1 ] ; then
			grep -E "^${comp}/dep11/Components-${arch}(\$|\\.)" .list
		fi
	done

	if [ "${SOURCES}" = 1 ] ; then
		grep -E "^${comp}/source/(Release|Sources)(\$|\\.)" .list

		if [ "${CONTENTS}" = 1 ] ; then
			grep -E "^${comp}/Contents-source" .list
		fi
	fi

	for lang in ${I18N} ; do
		grep -E "^${comp}/i18n/Translation-${lang}(\$|\\.)" .list
	done

	if [ "${DEP11}" = 1 ] ; then
		grep -E "^${comp}/dep11/" .list | grep -Ev "^${comp}/dep11/Components-"
	fi
done > .list.tmp

set -e

rm -f .list
sort -uV < .list.tmp > .list.fetch
rm -f .list.tmp

while read -r file ; do
	[ -n "${file}" ] || continue
	echo "# test ${file}" >&2
	_test_uri "${file}" || continue
	echo "# fetch ${file}" >&2
	_fetch_uri "${file}"
done < .list.fetch

## TODO: verify hashsums

cat > .list.put <<-EOF
Release
Release.gpg
InRelease
EOF
cat < .list.fetch >> .list.put

while read -r file ; do
	[ -n "${file}" ] || continue
	[ -f "${file}" ] || continue
	echo "# put ${file}" >&2
	_put_file "${file}"
done < .list.put

_cleanup
exit 0

) || { _cleanup ; exit 1 ; }
