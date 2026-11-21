// Exercise the shipped ExtendScript against document objects with the same
// bounds/stack conventions as the two Adobe hosts. Real rendering needs Adobe.
const { test } = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')

const script = fs.readFileSync(path.join(__dirname, '../assets/adobe-snapshot.jsx'), 'utf8')
function capture(app, api) {
  const files = new Map()
  class File {
    constructor(name) { this.fsName = name }
    open() { return true }
    write(text) { files.set(this.fsName, text); return true }
    close() {}
  }
  class Folder { constructor(name) { this.fsName = name } }
  const context = { app, File, Folder, SaveOptions: { DONOTSAVECHANGES: 2 }, ...api }
  vm.runInNewContext(script.replace('__OUTPUT_DIRECTORY__', '"/snap"'), context)
  assert.equal(files.get('/snap/error.txt'), undefined)
  return JSON.parse(files.get('/snap/manifest.json'))
}

test('Photoshop isolates visible layers in copies, preserves clipping and document pixel coordinates', () => {
  const px = n => ({ as: () => n })
  const bounds = ns => ns.map(px)
  const layer = (name, b, extra) => ({name, typename:'ArtLayer', kind:'NORMAL', bounds:bounds(b), visible:true, ...extra})
  const accent = layer('文字', [70,40,100,72])
  const group = {name:'排版', typename:'LayerSet', opacity:100, blendMode:0, visible:true, layers:[accent]}
  const mask = layer('剪贴纹理', [30,10,60,40], {grouped:true})
  const base = layer('底片', [-10,6,82,85])
  const hidden = layer('隐藏', [0,0,120,92], {visible:false})
  const saved = [], copies = []
  const copyLayer = l => ({...l, layers:l.layers?.map(copyLayer)})
  const source = {name:'样本.psd', width:px(120), height:px(92), layers:[group,hidden,mask,base]}
  source.duplicate = () => {
    const copy = {...source, layers:source.layers.map(copyLayer), mode:'rgb', bitsPerChannel:8,
      crop(b){this.width=px(b[2].as()-b[0].as());this.height=px(b[3].as()-b[1].as())},
      saveAs(f){ saved.push([f.fsName,this.width.as(),this.height.as(),this.layers.map(l=>l.visible)]) },
      close(){this.closed=true}
    }
    copies.push(copy);return copy
  }
  const app = {name:'Adobe Photoshop',documents:[source],activeDocument:source,displayDialogs:'all'}
  const m = capture(app, {UnitValue:n=>px(n),LayerKind:{SMARTOBJECT:'smart'},DocumentMode:{RGB:'rgb'},DialogModes:{NO:'no'},
    BitsPerChannelType:{EIGHT:8},PNGSaveOptions:class {},Extension:{LOWERCASE:1},BlendMode:{NORMAL:0,PASSTHROUGH:1},
    ActionReference:class {putIdentifier(){}},stringIDToTypeID:k=>k,executeActionGet:()=>({hasKey:()=>false})
  })
  assert.deepEqual(m.layers.map(l=>[l.name,l.group,l.x,l.y,l.w,l.h]),[['底片','',0,6,82,79],['文字','排版',70,40,30,32]])
  assert.deepEqual(saved[1].slice(1),[82,79,[false,false,true,true]])
  assert.equal(saved[0][0],'/snap/composite.png')
  assert.equal(app.displayDialogs,'all')
  assert.ok(copies.every(c=>c.closed))
  assert.deepEqual(source.layers.map(l=>l.visible),[true,false,true,true])
})

test('Illustrator snapshots only the active board and clamps the rendered bounds of each visible object', () => {
  const exported=[],closed=[]
  const source={name:'海报.ai',typename:'Document'}
  const parent={name:'排版',typename:'Layer',visible:true,parent:source}
  function leaf(name,b,extra) {
    return {name,visibleBounds:b,typename:'PathItem',parent,duplicate(){return {visibleBounds:b,translate(){}}},...extra}
  }
  const clipped=leaf('剪贴组',[90,190,170,100],{typename:'GroupItem',clipped:true,pageItems:[]})
  const title=leaf('标题',[125,171,160,150])
  source.artboards = [{name:'其它',artboardRect:[0,0,10,-10]},{name:'活动画板',artboardRect:[100,200,300,80]}]
  source.artboards.getActiveArtboardIndex=()=>1
  source.pageItems=[title,leaf('隐藏',[100,200,120,180],{hidden:true}),leaf('画板外',[0,0,8,-8]),clipped]
  source.exportFile=(f,_,o)=>exported.push([f.fsName,o.horizontalScale])
  const docs=[source]
  docs.add=()=>({layers:[{}],artboards:[{artboardRect:[0,500,500,0]}],
    exportFile(f,_,o){exported.push([f.fsName,o.horizontalScale,Array.from(this.artboards[0].artboardRect)])},
    close(){closed.push(true)}
  })
  const app={name:'Adobe Illustrator',activeDocument:source,documents:docs,coordinateSystem:'artboard'}
  const m=capture(app,{ExportOptionsPNG24:class {},ExportType:{PNG24:1},ElementPlacement:{PLACEATBEGINNING:1},
    BlendModes:{NORMAL:0},CoordinateSystem:{DOCUMENTCOORDINATESYSTEM:'document'}
  })
  assert.equal(app.coordinateSystem,'artboard')
  assert.deepEqual([m.width,m.height],[200,120])
  assert.deepEqual(m.layers.map(l=>[l.name,l.group,l.x,l.y,l.w,l.h]),[['剪贴组','排版',0,10,70,90],['标题','排版',25,29,35,21]])
  assert.deepEqual(exported[1],[ '/snap/layer-0', 100*(300/72), [10,500,80,410]])
  assert.equal(closed.length,2)
})
