#!/bin/bash

set -eu;
set -o pipefail;

until sudo systemctl status cloud-init | grep 'code=exited' | grep 'status=0/SUCCESS' >/dev/null; do
	date;
	sleep 1;
done

sudo systemctl status k3s || {
	echo "NOT !!!";
}
