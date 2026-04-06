#!/bin/sh
set -e
mkdir -p services
echo -e "\t\t\t<ul id='links'>" > services.html
echo "const iconUrls = [" > iconUrls.json
curl "https://configy.l42.eu/systems/http?fields=domain" -H "Accept: text/csv;header=absent" | while read service
	do
		serviceFile=services/$service.json
		echo $service
		infoResponse=$(curl "https://$service/_info" -s)
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
