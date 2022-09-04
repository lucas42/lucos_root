#!/bin/sh
mkdir -p build-output


echo -e "const localUrls = [\n\t\"/\"," > localUrls.json
ls public | awk '{print "\t\"/"$1"\""}' ORS=',\n' >> localUrls.json
echo -e "\n];" >> localUrls.json

cat localUrls.json > build-output/serviceworker.js
cat iconUrls.json >> build-output/serviceworker.js
cat templates/service-worker.js >> build-output/serviceworker.js


sed '/<services-list \/>/r'<(cat services.html) templates/index.html | sed 's/<services-list \/>//' > build-output/index.html