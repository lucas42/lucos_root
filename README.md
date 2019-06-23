# lucos_root

A homescreen for lucos modules.

## Dependencies
* docker
* docker-compose

## Build-time Dependencies
* Java
* [Google gson](https://code.google.com/p/google-gson/)
* [lucos_core](https://github.com/lucas42/lucos_core)

## Running
`nice -19 docker-compose up -d --no-build`


## Building
The build is configured to run in Dockerhub when a commit is pushed to the master branch in github.