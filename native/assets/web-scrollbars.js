// Attach rails to the viewport, so virtualized lists and their transforms do
// not move the scrollbar. Each axis follows the element actually scrolled.
const installRails = () => {
  const root = () => document.scrollingElement;
  const axes = ['y', 'x'];
  function createRail(vertical) {
    const rail = document.createElement('div');
    const thumb = document.createElement('div');
    rail.className = 'canvepp-rail';
    rail.dataset.axis = vertical ? 'y' : 'x';
    rail.tabIndex = 0;
    rail.setAttribute('role', 'scrollbar');
    rail.setAttribute('aria-label', vertical ? '垂直滚动' : '水平滚动');
    rail.setAttribute('aria-orientation', vertical ? 'vertical' : 'horizontal');
    rail.setAttribute('aria-valuemin', '0');
    rail.setAttribute('aria-valuemax', '100');
    rail.append(thumb);
    document.body.append(rail);
    let target, timer, grab;
    function layout(el) {
      const rect = el === root() ? {left:0, top:0} : el.getBoundingClientRect();
      const viewport = vertical ? el.clientHeight : el.clientWidth;
      const range = (vertical ? el.scrollHeight : el.scrollWidth) - viewport;
      const start = Math.max(0, vertical ? rect.top : rect.left);
      const end = Math.min(vertical ? innerHeight : innerWidth, (vertical ? rect.top : rect.left) + viewport);
      const length = end-start;
      const thumbLength = Math.min(length,Math.max(18,length*viewport/(range+viewport)));
      const travel = length-thumbLength;
      const offset = vertical ? el.scrollTop : el.scrollLeft;
      return {rect, start, length, viewport, range, thumbLength, travel, top: travel*offset/range};
    }
    function paint() {
      if (!target?.isConnected || !target.clientWidth) { rail.hidden=true; return; }
      const g=layout(target);
      rail.hidden = g.range<=0 || g.length<=0;
      if (rail.hidden) return;
      const edge = vertical ? g.rect.left+target.clientWidth : g.rect.top+target.clientHeight;
      Object.assign(rail.style,vertical
        ? {left:(Math.min(innerWidth,edge)-scrollStyle.edge)+'px',top:g.start+'px',height:g.length+'px'}
        : {left:g.start+'px',top:(Math.min(innerHeight,edge)-scrollStyle.edge)+'px',width:g.length+'px'});
      thumb.style[vertical?'height':'width']=g.thumbLength+'px';
      thumb.style.transform=vertical ? 'translateY('+g.top+'px)' : 'translateX('+g.top+'px)';
      rail.setAttribute('aria-valuenow',String(Math.round(g.top/g.travel*100)||0));
    }
    function show(el) {
      if (grab !== undefined && el !== target) return;
      target=el;
      paint();
      rail.classList.add('active');
      clearTimeout(timer);
      timer=setTimeout(()=>rail.classList.remove('active'),scrollStyle.idle);
    }
    const pos=e=>vertical?e.clientY:e.clientX;
    function move(e) {
      const g=layout(target);
      if (g.travel>0) target[vertical?'scrollTop':'scrollLeft']=(pos(e)-g.start-grab)*g.range/g.travel;
      paint();
    }
    rail.addEventListener('pointerdown',e=>{
      if(e.button!==0 || !target) return;
      const g=layout(target);
      grab=pos(e)-g.start-g.top;
      if(grab<0 || grab>g.thumbLength) grab=g.thumbLength/2;
      rail.setPointerCapture(e.pointerId);
      rail.focus({preventScroll:true});
      rail.classList.add('dragging');
      e.preventDefault(); move(e);
    });
    rail.addEventListener('pointermove',e=>{ if(grab!==undefined) move(e); });
    rail.addEventListener('lostpointercapture',()=>{grab=undefined;rail.classList.remove('dragging');show(target);});
    rail.addEventListener('keydown',e=>{
      const g=layout(target);
      const delta={Home:-g.range,End:g.range,PageUp:-g.viewport,PageDown:g.viewport,
        ArrowUp:-40,ArrowDown:40,ArrowLeft:-40,ArrowRight:40}[e.key];
      if(delta===undefined) return;
      e.preventDefault(); target[vertical?'scrollTop':'scrollLeft']+=delta;
    });
    rail.addEventListener('wheel',e=>{
      const g=layout(target);
      const delta=(vertical?e.deltaY:e.deltaX||e.deltaY)*(e.deltaMode===1?16:e.deltaMode===2?g.viewport:1);
      if(delta) {e.preventDefault();target[vertical?'scrollTop':'scrollLeft']+=delta;}
    },{passive:false});
    const overflow=el=>vertical?el.scrollHeight>el.clientHeight:el.scrollWidth>el.clientWidth;
    const near=(el,e)=>{
      const g=layout(el);
      const edge=vertical?g.rect.left+el.clientWidth:g.rect.top+el.clientHeight;
      return Math.abs(edge-(vertical?e.clientX:e.clientY))<=scrollStyle.edge+2;
    };
    target=root(); paint();
    return {rail,show,overflow,near,paint};
  }
  const rails=axes.map(axis=>createRail(axis==='y'));
  document.addEventListener('scroll',e=>{
    const el=e.target===document?root():e.target;
    if(el instanceof Element) rails.forEach(r=>{if(r.overflow(el))r.show(el);else r.paint();});
  },{passive:true,capture:true});
  document.addEventListener('pointermove',e=>{
    if (!(e.target instanceof Element) || e.target.closest('.canvepp-rail')) return;
    for(const r of rails) for(let el=e.target;el;el=el.parentElement){
      if(r.overflow(el) && r.near(el,e)){r.show(el);break;}
    }
  },{passive:true});
  window.addEventListener('resize',()=>rails.forEach(r=>r.paint()),{passive:true});
  new ResizeObserver(()=>rails.forEach(r=>r.paint())).observe(document.body);
};
installRails();
