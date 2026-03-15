ARG ELASTIC_VERSION=7.17.9

FROM elasticsearch:$ELASTIC_VERSION

ARG ELASTIC_VERSION

RUN \
  ./bin/elasticsearch-plugin install https://artifacts.elastic.co/downloads/elasticsearch-plugins/analysis-icu/analysis-icu-${ELASTIC_VERSION}.zip && \
  ./bin/elasticsearch-plugin install https://github.com/vhyza/elasticsearch-analysis-lemmagen/releases/download/v${ELASTIC_VERSION}/elasticsearch-analysis-lemmagen-${ELASTIC_VERSION}-plugin.zip && \
  cd config && \
  curl -L -O https://github.com/vhyza/lemmagen-lexicons/raw/refs/heads/master/free/lexicons/sl.lem
