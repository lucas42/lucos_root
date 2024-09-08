#!/bin/sh
set -e
mkdir -p services
echo -e "\t\t\t<ul id='links'>" > services.html
echo "const iconUrls = [" > iconUrls.json
cat service-list | while read service
	do
		serviceFile=services/$service.json
		echo $service
		curl "https://$service/_info" -s | \
		jq "select( .show_on_homepage == true) | {icon: (\"https://$service\"+.icon),network_only,start_url: (\"https://$service\"+(.start_url // \"/\")),title}" > $serviceFile
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