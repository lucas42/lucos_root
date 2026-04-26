#!/bin/sh
set -e

# Per-attempt timeout for /_info fetch (seconds).  Bounds each curl call so
# a hung connection cannot block the build indefinitely.
PER_REQUEST_TIMEOUT=10
# Maximum number of fetch attempts per service before giving up.
MAX_ATTEMPTS=4

# Fetch /_info from a service, retrying on transient failures (curl errors
# and HTTP 5xx).  Writes the response body to stdout; exits non-zero on
# permanent failure or exhausted retries.
#
# We deliberately do NOT retry on a non-5xx response with a non-JSON body —
# that indicates a permanent misconfiguration in the target service rather
# than a transient blip, and the build should fail fast in that case
# (caught by the jq parse below).
fetch_service_info() {
	service=$1
	attempt=1
	backoff=2
	while [ "$attempt" -le "$MAX_ATTEMPTS" ]; do
		curl_rc=0
		# -w '\n%{http_code}' appends the HTTP status code on its own
		# line after the body so we can decide whether to retry.
		response=$(curl "https://$service/_info" -s \
			-H "User-Agent: lucos_root" \
			--max-time "$PER_REQUEST_TIMEOUT" \
			-w '\n%{http_code}') || curl_rc=$?

		if [ "$curl_rc" -eq 0 ]; then
			status=$(printf '%s' "$response" | tail -n1)
			body=$(printf '%s' "$response" | sed '$d')
			if [ "$status" -lt 500 ] || [ "$status" -ge 600 ]; then
				printf '%s' "$body"
				return 0
			fi
			echo "Attempt $attempt/$MAX_ATTEMPTS: HTTP $status from $service" >&2
		else
			echo "Attempt $attempt/$MAX_ATTEMPTS: curl exit $curl_rc fetching $service" >&2
		fi

		if [ "$attempt" -lt "$MAX_ATTEMPTS" ]; then
			sleep "$backoff"
			backoff=$((backoff * 2))
		fi
		attempt=$((attempt + 1))
	done

	echo "Error: $service did not return a non-5xx response after $MAX_ATTEMPTS attempts" >&2
	return 1
}

mkdir -p services
echo -e "\t\t\t<ul id='links'>" > services.html
echo "const iconUrls = [" > iconUrls.json
curl "https://configy.l42.eu/systems/http?fields=domain" -H "Accept: text/csv;header=absent" -H "User-Agent: lucos_root" | while read service
	do
		serviceFile=services/$service.json
		echo $service
		if ! infoResponse=$(fetch_service_info "$service"); then
			exit 1
		fi
		if ! echo "$infoResponse" | jq "select( .show_on_homepage == true) | {icon: (\"https://$service\"+.icon),network_only,start_url: (\"https://$service\"+(.start_url // \"/\")),title}" > $serviceFile; then
			echo "Error: Failed to parse /_info response from $service" >&2
			echo "Response (first 200 chars): $(echo "$infoResponse" | head -c 200)" >&2
			exit 1
		fi
		if [[ ! -s $serviceFile ]]
		then
			rm $serviceFile
			continue
		fi
		echo -en "\t\t\t\t" >> services.html
		jq -r '"<li class=\""+(if .network_only then "networkonly" else "" end)+"\"><a href=\""+.start_url+"\"><img src=\""+.icon+"\" alt=\""+.title+"\" title=\""+.title+"\" /></a></li>"' $serviceFile >> services.html
		echo -en "\t" >> iconUrls.json
		jq .icon $serviceFile | sed 's/$/,/g' >> iconUrls.json
	done

echo -e "\t\t\t</ul>" >> services.html
echo "];" >> iconUrls.json
