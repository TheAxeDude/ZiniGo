# ZiniGo

A tool written in Go for saving (legally purchased) magazines from Zinio as DRM-free PDFs. 

## Precompiled binaries

ZiniGo can be downloaded for Windows and Linux at https://github.com/TheAxeDude/ZiniGo/tree/master/built

## Usage

./zinigo -u=Username -p=Password [-c=/path/to/chrome/executable]

You can also add these properties to a config file in the working directory, instead of passing them in manually. See the sample at https://github.com/TheAxeDude/ZiniGo/blob/master/config.json

You can use `-playwright=true` to use Playwright to run the tests. It'll download a browser in the background.

## Requirements

No specific dependencies need to be installed, however you can specify the location of the chrome executable to be used rendering the PDF.

If using a pre-installed chrome, the command `google-chrome` should work, or a location of the Chrome executable should be passed in via the `-c` parameter.

## How it works
ZiniGo logs into Zinio, and pulls a list of all issues purchased. 

Each page is available as an SVG, which is then injected into an HTML page (based on template.html).

PlayWright (or google-chrome) is then used to print the page to PDF, and all pages are combined into a single PDF.

## Building
Build for linux & windows on windows using the powershell script in buildscripts
Build for linux & windows on macos using the darwin.sh script in buildscripts

