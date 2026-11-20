package settings

import "github.com/nsgo-dev/nsgo/core"

// riverScene is the design's river2 picture under the Subtitle Style page's preview, 358 by 200: a sky over a range of mountains, a sun, and a river winding down through green hills. It is drawn in the colors that show at 0.9 over the preview's black, as the design lays it.
var riverScene = core.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 358 200" width="358" height="200">
<defs>
<linearGradient id="sky" x1="0" y1="0" x2="0" y2="1">
<stop offset="0" stop-color="#8FB8D1"/>
<stop offset="0.55" stop-color="#BDCBC6"/>
</linearGradient>
<linearGradient id="river" x1="0" y1="0" x2="0" y2="1">
<stop offset="0" stop-color="#A9D2DE"/>
<stop offset="1" stop-color="#8FB4C0"/>
</linearGradient>
</defs>
<rect width="358" height="200" fill="url(#sky)"/>
<circle cx="266" cy="64" r="19" fill="#FFF2CC"/>
<polygon points="0,122 67,78 123,105 190,61 257,100 313,72 358,93 358,200 0,200" fill="#6A8EA1"/>
<path d="M0,156 C60,140 120,136 180,146 S300,150 358,134 L358,200 L0,200 Z" fill="#3D6A5B"/>
<path d="M232,124 C242,122 250,123 254,126 C232,138 204,148 214,164 C222,178 204,192 194,200 L170,200 C182,190 198,178 188,164 C178,148 206,136 232,124 Z" fill="url(#river)"/>
<ellipse cx="179" cy="194" rx="7" ry="3" fill="#7F8A8F"/>
</svg>`))
