FROM lucas42/lucos_navbar:latest as navbar
FROM httpd:2.4-alpine

RUN echo "Include conf/vhost.conf" >> /usr/local/apache2/conf/httpd.conf
COPY vhost.conf /usr/local/apache2/conf/
COPY public/ /usr/local/apache2/lucos_root
COPY --from=navbar lucos_navbar.js /usr/local/apache2/lucos_root/
COPY sw-template.js /tmp/
RUN sed '/urls = \[/r'<(ls  /usr/local/apache2/lucos_root/ | awk '{print "\t\"/"$1"\""}' ORS=',\n') /tmp/sw-template.js > /usr/local/apache2/lucos_root/serviceworker.js
RUN rm /tmp/sw-template.js