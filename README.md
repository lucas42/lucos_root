#lucos_root

A homescreen for lucos modules.

##Dependencies
* Java
* [Google gson](https://code.google.com/p/google-gson/)
* [lucos_core](https://github.com/lucas42/lucos_core)

##Installation
To build the project, run *./build.sh*

##Running
The web server is designed to be run within lucos_services, but can be run standalone by running ```java -cp .:bin:../lib/java/* Server``` from the root of the project. It currently runs on port 8003.
