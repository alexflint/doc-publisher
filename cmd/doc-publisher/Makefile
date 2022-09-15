default: out/sample_for_tinkering.pdf

force:

out/sample_for_tinkering.pdf: out/sample_for_tinkering.tex force
	xelatex --output-directory=/tmp out/sample_for_tinkering.tex
	mkdir -p out
	mv /tmp/sample_for_tinkering.pdf out

setup:
	sudo apt install \
		texlive-xetex \
		texlive-fonts-extra \
		texlive-publishers \
		texlive-bibtex-extra \
		texlive-xetex


# Secret encryption and decryption
encrypt-secrets:
	echo "Please enter the password from lastpass under 'doc-publisher secrets'..."
	go run ./cmd/crypt/*.go --encrypt secrets/*.json

decrypt-secrets:
	echo "Please enter the password from bitwarden under 'doc-publisher secrets'..."
	go run ./cmd/crypt/*.go --decrypt secrets/*.encrypted
