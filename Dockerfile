FROM lucas42/lucos_navbar:latest as navbar

FROM alpine:latest as build
RUN apk add curl jq
RUN mkdir {public,templates}
COPY build-config .
COPY templates templates/
COPY public public/

RUN ./fetch-service-info.sh
RUN ./populate-templates.sh


FROM httpd:2.4-alpine

WORKDIR /usr/local/apache2/lucos_root/
RUN echo "Include conf/vhost.conf" >> /usr/local/apache2/conf/httpd.conf
COPY vhost.conf /usr/local/apache2/conf/
COPY public/ .
COPY --from=navbar lucos_navbar.js .
COPY --from=build build-output .