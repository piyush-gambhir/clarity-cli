.PHONY: build test vet install docs
build test vet install docs:
	$(MAKE) -C cli-go $@
