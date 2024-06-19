.PHONY: all check release upstream $(wildcard tests/Dockerfile*)

all:
	docker build . -t btidor-syntax-dev

check: $(wildcard tests/Dockerfile*)

tests/Dockerfile*:
	docker build -f $@ tests/ --no-cache
	docker build -f $@ tests/
	docker build -f $@ tests/

release:
	@bash -c '[[ "$V" =~ ^[0-9]+\.[0-9]+\.[0-9]+$$ ]] || \
			(echo "usage: make release V=x.y.z"; exit 1)'
	@bash -c 'read -p "Release v$V? " -n 1 -r && echo && \
			([[ $${REPLY^^} == "Y" ]] || exit 2)'
	git tag -s "v$V" -m "syntax@v$V"
	git push origin "v$V"

upstream:
	@bash -c '[[ -n "${V}" ]] || \
		(echo "usage: make upstream V=x.y.z"; exit 1)'
	rm -rf dockerfile frontend go.mod
	curl -L "https://github.com/moby/buildkit/archive/refs/tags/dockerfile/$V.tar.gz" | \
		tar zxv --wildcards "buildkit-*/frontend/dockerfile" "buildkit-*/go.mod" --strip-components=1
	mv frontend/dockerfile/ .
	mv go.mod dockerfile/
	rm -r frontend dockerfile/docs dockerfile/linter/docs dockerfile/parser
	sed -i -e 's#github.com/moby/buildkit/frontend/dockerfile/cmd/dockerfile-frontend#github.com/btidor/syntax#g' \
		dockerfile/cmd/dockerfile-frontend/version.go
	sed -i -e 's#0.0.0+unknown#$V#g' dockerfile/cmd/dockerfile-frontend/version.go
	find dockerfile/ -type f -exec sed -i -e \
		's#github.com/moby/buildkit/frontend/dockerfile#github.com/btidor/syntax/dockerfile#g' {} \;
	sed -i -e 's#module github.com/moby/buildkit#module github.com/btidor/syntax/dockerfile#g' dockerfile/go.mod
	find dockerfile/ -type f -exec sed -i -e \
		's#github.com/btidor/syntax/dockerfile/parser#github.com/moby/buildkit/frontend/dockerfile/parser#g' {} \;
	cd dockerfile && go mod tidy && go fmt ./...
