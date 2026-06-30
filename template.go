package main

import "html/template"

// pageTemplate renders the list of peers from a subs.yaml file.
var pageTemplate = template.Must(template.New("page").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
body { font-family: system-ui, sans-serif; margin: 2rem auto; width: fit-content; min-width: min(450px, 100%); max-width: 600px; padding: 0 1rem; box-sizing: border-box; background: #0f1115; color: #e6e6e6; }
h1 { font-weight: 600; text-align: center; margin-bottom: 1.5rem; }
.title-wrap { position: relative; text-align: center; }
.title-wrap h1 { display: inline-block; cursor: default; }
.desc-tip { position: absolute; left: 50%; top: 100%; transform: translateX(-50%); max-width: min(90vw, 360px); padding: .5rem .75rem; border-radius: 8px; border: 1px solid #2a2e37; background: #222733; color: #cfd6e4; font-size: .9rem; line-height: 1.35; text-align: center; opacity: 0; visibility: hidden; pointer-events: none; z-index: 10; transition: opacity .2s ease, visibility .2s ease; }
.title-wrap h1:hover + .desc-tip { opacity: 1; visibility: visible; transition-delay: 500ms; }
.peer { border: 1px solid #2a2e37; border-radius: 10px; padding: 1.5rem 1.25rem; margin: 1rem 0; background: #171a21; }
.peer-head { display: flex; flex-direction: column; gap: .25rem; margin-bottom: .85rem; text-align: center; }
.peer-title, .peer-comment { width: 100%; overflow: hidden; -webkit-mask-image: linear-gradient(90deg, #000 calc(100% - 28px), transparent 100%); mask-image: linear-gradient(90deg, #000 calc(100% - 28px), transparent 100%); }
.peer-title span, .peer-comment span { display: inline-block; max-width: 100%; white-space: nowrap; vertical-align: bottom; }
.peer-title { font-weight: 600; font-size: 1.05rem; }
.peer-comment { color: #8a93a5; font-size: .9rem; }
.actions { display: flex; gap: .5rem; flex-wrap: wrap; justify-content: center; }
.btn { display: inline-flex; align-items: center; padding: .45rem .8rem; border-radius: 8px; border: 1px solid #2a2e37; background: #222733; color: #e6e6e6; font: inherit; font-size: .9rem; cursor: pointer; text-decoration: none; }
.btn:hover:not(:disabled) { background: #2b313f; }
.btn:disabled { opacity: .45; cursor: not-allowed; }
.modal { position: fixed; inset: 0; background: rgba(0,0,0,.7); display: none; align-items: center; justify-content: center; padding: 1rem; }
.modal.open { display: flex; }
.modal-card { background: #fff; padding: 1rem; border-radius: 12px; }
.modal-card img { display: block; width: 384px; height: 384px; max-width: 90vw; max-height: 90vw; image-rendering: pixelated; }
</style>
</head>
<body>
{{if .Description}}<div class="title-wrap"><h1>{{.Title}}</h1><div class="desc-tip">{{.Description}}</div></div>{{else}}<h1>{{.Title}}</h1>{{end}}
{{range .Peers}}
<div class="peer">
  <div class="peer-head">
    <div class="peer-title"><span>{{.Title}}</span></div>
    {{if .Comment}}<div class="peer-comment"><span>{{.Comment}}</span></div>{{end}}
  </div>
  <div class="actions">
    <a class="btn" href="/{{$.Subid}}/{{.ConfigFile}}" download>Download config</a>
    <button class="btn" type="button" data-qr="/{{$.Subid}}/{{.ConfigFile}}?qr" onclick="showQR(this)"{{if not .ShowQR}} disabled{{end}}>QR code</button>
    <button class="btn" type="button" data-link="{{.Link}}" onclick="copyLink(this)"{{if not .Link}} disabled{{end}}>Copy VPN link</button>
  </div>
</div>
{{else}}
<p style="text-align: center; color: #8a93a5;">No configurations yet.</p>
{{end}}
<div class="modal" id="qr-modal" onclick="if(event.target===this)closeQR()">
  <div class="modal-card"><img id="qr-img" alt="QR code"></div>
</div>
<script>
function copyLink(btn) {
  navigator.clipboard.writeText(btn.dataset.link).then(function () {
    var prev = btn.textContent;
    btn.textContent = "Copied!";
    setTimeout(function () { btn.textContent = prev; }, 1200);
  });
}
function showQR(btn) {
  document.getElementById("qr-img").src = btn.dataset.qr;
  document.getElementById("qr-modal").classList.add("open");
}
function closeQR() {
  document.getElementById("qr-modal").classList.remove("open");
}
</script>
</body>
</html>
`))

// pageData is the view model passed to pageTemplate.
type pageData struct {
	Title       string
	Description string
	Subid       string
	Peers       []Peer
}
