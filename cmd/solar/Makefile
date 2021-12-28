BINARYNAME=solar
bin: docker/work/${BINARYNAME}

clean:
	rm -rf docker/work

docker/work:
	mkdir $@

docker/work/ca-certificates.crt: /etc/ssl/certs/ca-certificates.crt
	cp $< $@

docker/work/zoneinfo.zip: /usr/local/go/lib/time/zoneinfo.zip
	cp $< $@

docker/work/${BINARYNAME}: main.go docker/work
	CGO_ENABLED=0 GOARCH=arm go build -tags netgo -ldflags '-w -s' -o $@ .

container: bin docker/Dockerfile docker/work/ca-certificates.crt docker/work/zoneinfo.zip
	docker build -t registry:31320/solar:latest docker/

push: container
	docker push registry:31320/solar:latest