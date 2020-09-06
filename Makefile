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
