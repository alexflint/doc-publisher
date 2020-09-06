default: out/main.pdf

bib: out/main.bbl

force:

out/main.pdf: main.tex force
	mkdir -p out
	xelatex --output-directory=out main.tex

out/main.bbl: library.bib force
	mkdir -p out
	biber --output-directory=out main.bcf

clean:
	rm -f out/*

setup:
	sudo apt install \
		texlive-xetex \
		texlive-fonts-extra \
		texlive-publishers \
		texlive-bibtex-extra \
		texlive-xetex
