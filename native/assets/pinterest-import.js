(() => {
  const scrollStyle = __CANVEPP_SCROLL__;
  const allowed = ['pinterest.com', 'www.pinterest.com', 'www.pinterest.co.uk', 'www.pinterest.jp', 'www.pinterest.ca', 'www.pinterest.de', 'www.pinterest.fr'];
  if (window.top !== window || location.protocol !== 'https:' || !allowed.includes(location.hostname) || window.__canveppPins) return;
  window.__canveppPins = true;
  const seen = new WeakSet();
  const candidate = image => {
    const options = (image.srcset || '').split(',').map(s => s.trim().split(/\s+/)).filter(s => s[0]);
    options.sort((a,b) => (parseFloat(b[1])||0) - (parseFloat(a[1])||0));
    return options[0]?.[0] || image.currentSrc || image.src;
  };
  // A native browser drag can cross from the embedded page into the canvas.
  document.addEventListener('dragstart', event => {
    if (!event.isTrusted || !event.dataTransfer || !(event.target instanceof Element)) return;
    const anchor = event.target.closest('a[href*="/pin/"]');
    const img = event.target.closest('img') || anchor?.querySelector('img');
    const pin = anchor?.href || location.href;
    if (!img || !/^https:\/\/[^/]+\/pin\/\d+\/?(?:[?#]|$)/.test(pin)) return;
    const data = new URLSearchParams({image:candidate(img), pin, name:img.alt.slice(0,140)});
    event.dataTransfer.setData('text/plain','canvepp-pinterest://import?' + data);
    event.dataTransfer.effectAllowed = 'copy';
  }, true);
  // The official Google button lives inside a cross-origin iframe. In the
  // embedded library, offer the collection connection action in the same form slot.
  // The host handles this fixed destination on a real click or keyboard press.
  const installGoogleLink = () => {
    document.querySelectorAll('div[data-test-id="google-connect-button"]').forEach(container => {
      if (container.querySelector(':scope > .canvepp-google-login')) return;
      const link = document.createElement('a');
      link.className = 'canvepp-google-login';
      link.href = 'https://www.pinterest.com/connect/google/';
      link.target = '_blank';
      link.rel = 'noopener noreferrer';
      link.textContent = document.documentElement.lang.toLowerCase().startsWith('zh')
        ? '用 Google 连接收藏' : 'Connect with Google';
      container.appendChild(link);
    });
  };
  const scan = () => {
    installGoogleLink();
    document.querySelectorAll('a[href*="/pin/"]').forEach(anchor => {
      const img = anchor.querySelector('img');
      if (!img || seen.has(anchor) || img.getBoundingClientRect().width < 90) return;
      const imageURL = candidate(img);
      try { if (new URL(imageURL).hostname !== 'i.pinimg.com') return; } catch { return; }
      seen.add(anchor);
      const button = document.createElement('button');
      button.type = 'button';
      button.setAttribute('aria-label', '导入当前画布');
      const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
      svg.setAttribute('viewBox', '0 0 24 24');
      svg.setAttribute('width', '16');
      svg.setAttribute('height', '16');
      svg.setAttribute('aria-hidden', 'true');
      const plus = document.createElementNS(svg.namespaceURI, 'path');
      plus.setAttribute('d', 'M5 12h14M12 5v14');
      plus.setAttribute('stroke', 'currentColor');
      plus.setAttribute('stroke-width', '2');
      plus.setAttribute('stroke-linecap', 'round');
      svg.append(plus);
      button.append(svg);
      button.className = 'canvepp-import-pin';
      anchor.classList.add('canvepp-pin');
      if (getComputedStyle(anchor).position === 'static') anchor.style.position = 'relative';
      anchor.appendChild(button);
      button.addEventListener('click', event => {
        event.preventDefault(); event.stopPropagation();
        if (!event.isTrusted) return;
        const current = anchor.querySelector('img'); if (!current) return;
        const data = new URLSearchParams({image:candidate(current), pin:anchor.href, name:(current.alt||'Pinterest 图片').slice(0,140)});
        location.href = 'canvepp-pinterest://import?' + data;
      }, true);
    });
  };
  let scheduled = false;
  const schedule = () => { if (scheduled) return; scheduled = true; setTimeout(() => { scheduled = false; scan(); }, 350); };
  const start = () => {
    const style = document.createElement('style');
    style.textContent = `
      * { scrollbar-width: none !important; }
      *::-webkit-scrollbar { display:none !important; }
      .canvepp-rail { position:fixed; z-index:2147483646; touch-action:none; cursor:pointer; background:transparent; }
      .canvepp-rail[hidden] { display:none !important; }
      .canvepp-rail[data-axis=y] { width:${scrollStyle.edge}px; }
      .canvepp-rail[data-axis=x] { height:${scrollStyle.edge}px; }
      .canvepp-rail > div { position:absolute; border-radius:2px; opacity:0; pointer-events:none; background:rgba(99,99,99,.48); }
      .canvepp-rail[data-axis=y] > div { left:calc(50% - ${scrollStyle.width/2}px); width:${scrollStyle.width}px; }
      .canvepp-rail[data-axis=x] > div { top:calc(50% - ${scrollStyle.width/2}px); height:${scrollStyle.width}px; }
      .canvepp-rail:is(.active,.dragging,:hover,:focus-visible) > div { opacity:1; }
      div[data-test-id="google-connect-button"]:has(> .canvepp-google-login) > :not(.canvepp-google-login) { display: none !important; }
      .canvepp-google-login {
        display: flex; align-items: center; justify-content: center; box-sizing: border-box;
        width: 100%; min-height: 40px; padding: 6px 10px; border-radius: 8px;
        background: #f1f1f1; color: #272727; cursor: pointer; text-decoration: none;
        font: 500 14px/20px system-ui;
      }
      .canvepp-google-login:hover { background: #e6e6e6; }
      .canvepp-google-login:focus-visible { outline: 2px solid currentColor; outline-offset: 2px; }
      .canvepp-import-pin {
        position: absolute; left: 10px; bottom: 10px; z-index: 20;
        width: 32px; height: 32px; padding: 0;
        border: 0; border-radius: 6px;
        display: grid; place-items: center; background: #fff; color: #222;
        cursor: pointer; opacity: 0;
      }
      .canvepp-pin:hover > .canvepp-import-pin,
      .canvepp-pin:focus-within > .canvepp-import-pin { opacity: 1; }
      .canvepp-import-pin:hover { background: #eaeaea; }
      .canvepp-import-pin:focus-visible { outline: 2px solid #272727; outline-offset: 2px; }
      .canvepp-import-pin:hover::after, .canvepp-import-pin:focus-visible::after {
        content: attr(aria-label); position: absolute; left: 0; bottom: calc(100% + 5px);
        white-space: nowrap; padding: 4px 8px; border-radius: 5px;
        background: #272727; color: #fff; font: 12px/18px system-ui; pointer-events: none;
      }
      @media (prefers-color-scheme: dark) {
        .canvepp-google-login { background: #333; color: #eee; }
        .canvepp-google-login:hover { background: #404040; }
        .canvepp-rail > div { background: rgba(180,180,180,.48); }
        .canvepp-import-pin { background: #292929; color: #f3f3f3; }
        .canvepp-import-pin:hover { background: #454545; }
        .canvepp-import-pin:focus-visible { outline-color: #eee; }
        .canvepp-import-pin:hover::after, .canvepp-import-pin:focus-visible::after { background: #eee; color: #222; }
      }
      @media (hover: none) { .canvepp-import-pin { opacity: 1; } }
    `;
    document.head.appendChild(style);
    __CANVEPP_RAILS__
    scan();
    new MutationObserver(schedule).observe(document.documentElement,{childList:true,subtree:true});
  };
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', start, {once:true}); else start();
  document.addEventListener('scroll',schedule,{passive:true,capture:true});
})();
