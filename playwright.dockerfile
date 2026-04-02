FROM node:24.10-bookworm

# Set trust for our CA root certificate.
# See: https://github.com/microsoft/playwright/issues/4785#issuecomment-1611570074

RUN apt-get update -q -q && \
  apt-get install --yes --force-yes libnss3-tools && \
  apt-get clean && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/* ~/.cache ~/.npm

COPY test-e2e-rootCA.pem /usr/local/share/ca-certificates/cacerts.crt
RUN update-ca-certificates && \
  mkdir -p $HOME/.pki/nssdb && \
  certutil --empty-password -d $HOME/.pki/nssdb -N && \
  certutil -d sql:$HOME/.pki/nssdb -A -t "C,," -n cacerts.crt -i /usr/local/share/ca-certificates/cacerts.crt

ENV NODE_EXTRA_CA_CERTS=/usr/local/share/ca-certificates/cacerts.crt

# Install playwright.

COPY . /src/peerdb
WORKDIR /src/peerdb

RUN npm ci --audit=false
RUN npm run test-e2e-install

ENV PEERDB_URL=
ENV LINK_PUBLISH_JOB_ID=
ENV UPDATE_SCREENSHOTS=

CMD ["npm", "run", "test-e2e"]
