FROM lucas42/lucos_navbar:latest as navbar
FROM httpd:2.4-alpine

RUN echo "Include conf/vhost.conf" >> /usr/local/apache2/conf/httpd.conf
COPY vhost.conf /usr/local/apache2/conf/
COPY public/ /usr/local/apache2/lucos_root
COPY --from=navbar lucos_navbar.js /usr/local/apache2/lucos_root/