FROM openjdk:13-alpine

WORKDIR /web/lucos/root

# Legacy method of installing resources was using the lucos_core library - installed in a relative location on the file system
RUN apk add git
RUN git clone https://github.com/lucas42/lucos_core.git /web/lucos/core

RUN mkdir -p /web/lucos/lib/java/

RUN wget "https://repo1.maven.org/maven2/com/google/code/gson/gson/2.8.1/gson-2.8.1.jar" -O /web/lucos/lib/java/gson-2.8.1.jar

COPY . .

RUN ./build.sh

CMD [ "java", "-cp", ".:bin:../lib/java/*", "Server" ]