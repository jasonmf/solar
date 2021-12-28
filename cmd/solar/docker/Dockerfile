FROM scratch
ADD work/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
ADD work/zoneinfo.zip /zoneinfo.zip
ADD work/solar /solar
ENV ZONEINFO=/zoneinfo.zip
ENTRYPOINT ["/solar"]