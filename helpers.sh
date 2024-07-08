HTTP="$(which httpie)"
JQ="$(which jq)"

SERVER="http://localhost:8080"

_dep() {
	if [[ -z $HTTP ]]; then
		echo "httpie not found in path."
		exit 1
	fi
	if [[ -z $JQ ]]; then
		echo "jq not found in path."
		exit 1
	fi
}

dbox() {
	local subcommand
	subcommand=$1
	shift
	case $subcommand in
	"install")
		_installPup "$@"
		;;
	"listavailable")
		_listavailable "$@"
		;;
	"bootstrap")
		_getbootstrap "$@"
		;;
	*)
		echo "Invalid subcommand"
		;;
	esac
}

_listavailable() {
	http get "$SERVER/bootstrap/" | jq '.manifests[].available[].id'
}
_getbootstrap() {
	http get "$SERVER/bootstrap/"
}

_installPup() {
	http get "$SERVER/pup/$1/install"
}
