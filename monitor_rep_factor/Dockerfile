FROM %%PROVIDER%%/%%DOMAIN%%/alpine-java:8

ENV KAFKA_HOME /opt/kafka/kafka_2.10-0.10.2.0
ENV PATH ${PATH}:${KAFKA_HOME}/bin
ENV GOPATH /opt/gotools

WORKDIR /opt/kafka

RUN apk add --update bash curl docker openssh jq go sudo gcc musl-dev && \
  curl -O http://mirrors.koehn.com/apache/kafka/0.10.2.0/kafka_2.10-0.10.2.0.tgz && \
  tar -zxf /opt/kafka/kafka_2.10-0.10.2.0.tgz && \
  rm /opt/kafka/kafka_2.10-0.10.2.0.tgz && \
  mkdir $GOPATH && \
  go get gopkg.in/gomail.v2 && \
  go get github.com/bluele/slack

ADD replication-factor.go /opt/gotools/src/
