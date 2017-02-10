DIR := ${CURDIR}

install:
	go install github.com/timtadh/dynagrok

clientbad:
	git -C examples/src/dynagrok/examples/shapes checkout dynagrokfaulty
	dynagrok -g $(DIR)/examples -d $(DIR) instrument -w /tmp/work --keep-work -o clientbad.instr dynagrok/examples/shapes/client
	git -C  examples/src/dynagrok/examples/shapes checkout dynagrok

clientgood:
	git -C examples/src/dynagrok/examples/shapes checkout dynagrok
	dynagrok -g $(DIR)/examples -d $(DIR) instrument -w /tmp/work --keep-work -o clientgood.instr dynagrok/examples/shapes/client

run: install clientgood

html:
	dynagrok -g /home/koby/dev/repos/dynagrok/examples -d /home/koby/dev/repos/dynagrok instrument -w /tmp/work --keep-work -o html.instr dynagrok/examples/html/main

example:
	dynagrok -g $(DIR)/examples -d $(DIR) instrument -w /tmp/work --keep-work -o example.instr dynagrok/examples/shapes/client

tmp:
	./dynagrok -r ~/dev/repos/go-research -d /home/koby/dev/repos/dynagrok -g /home/koby/dev/repos/dynagrok/examples objectstate -m Move --keep-work -w /tmp/work dynagrok/examples/shapes/client
