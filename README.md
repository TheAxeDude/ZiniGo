# ZiniGo

A tool written in Go for saving (legally purchased) magazines from Zinio. 

## Precompiled binaries

ZiniGo can be downloaded for Windows and Linux at https://github.com/TheAxeDude/ZiniGo/tree/master/built

## Usage

./zinigo -u=Username -p=Password

## Requirements
Google chrome installed, and accessible via the command `google-chrome`

## How it works
ZiniGo logs into Zinio, and pulls a list of all issues purchased. 

Each page is available as an SVG, which is then injected into n HTML page (based on template.html).

google-chrome is then used to print the page to PDF, and all pages are combined into a single PDF.

