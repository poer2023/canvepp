// ExtendScript (ES3), shared by Photoshop and Illustrator. The host bridge
// supplies a private output directory. Only disposable documents are changed.
(function () {
    var output = new Folder(__OUTPUT_DIRECTORY__);
    var rows = [], pixelLimit = 39900000;
    function file(name) { return new File(output.fsName + "/" + name); }
    function write(name, text) {
        var f = file(name); f.encoding = "UTF-8";
        if (!f.open("w")) throw Error("Cannot write " + name);
        try { if (!f.write(text)) throw Error("Cannot write " + name); } finally { f.close(); }
    }
    function quote(value) {
        return '"' + String(value).replace(/[\\"\x00-\x1f]/g, function (ch) {
            return "\\u" + ("0000" + ch.charCodeAt(0).toString(16)).slice(-4);
        }) + '"';
    }
    function nextFile() {
        return "layer-" + rows.length + ".png";
    }
    function record(path, name, group, x, y, w, h) {
        rows.push('{"file":' + quote(path) + ',"name":' + quote(name) + ',"group":' + quote(group) +
            ',"x":' + x + ',"y":' + y + ',"w":' + w + ',"h":' + h + '}');
    }
    function groupPath(parent, name) { return parent ? parent + " / " + name : name; }
    function scaleFor(w, h, wanted) {
        return Math.min(wanted, Math.sqrt(pixelLimit / (Math.max(1,w) * Math.max(1,h))));
    }
    function pixels(n) { return n.as("px"); }
    function smartScale(doc, layer) {
        if (layer.typename === "LayerSet") {
            var largest=1;
            for (var i=0;i<layer.layers.length;i++)
                if (layer.layers[i].visible) largest=Math.max(largest,smartScale(doc,layer.layers[i]));
            return largest;
        }
        if (layer.typename !== "ArtLayer" || layer.kind !== LayerKind.SMARTOBJECT) return 1;
        try {
            doc.activeLayer = layer;
            var id = stringIDToTypeID, ref = new ActionReference();
            ref.putIdentifier(id("layer"), layer.id);
            var more = executeActionGet(ref).getObjectValue(id("smartObjectMore"));
            var size = more.getObjectValue(id("size")), t = more.getList(id("transform"));
            function length(i, j) {
                var x=t.getDouble(i)-t.getDouble(j), y=t.getDouble(i+1)-t.getDouble(j+1);
                return Math.max(1, Math.sqrt(x*x+y*y));
            }
            function dimension(key) {
                try {return size.getDouble(id(key));} catch (_) {return size.getUnitDoubleValue(id(key));}
            }
            return Math.max(1, dimension("width")/length(0,2), dimension("height")/length(0,6));
        } catch (_) { return 1; }
    }
    function savePS(doc, name, wanted) {
        var w=pixels(doc.width), h=pixels(doc.height), scale=scaleFor(w,h,wanted);
        if (Math.abs(scale-1)>.0001)
            doc.resizeImage(UnitValue(Math.max(1,Math.floor(w*scale)),"px"), UnitValue(Math.max(1,Math.floor(h*scale)),"px"), null, ResampleMethod.BICUBIC);
        if (doc.mode !== DocumentMode.RGB) doc.changeMode(ChangeMode.RGB);
        if (doc.bitsPerChannel !== BitsPerChannelType.EIGHT) doc.bitsPerChannel = BitsPerChannelType.EIGHT;
        doc.saveAs(file(name), new PNGSaveOptions(), true, Extension.LOWERCASE);
    }
    function splitPSGroup(group) {
        if (group.opacity !== 100 || (group.blendMode !== BlendMode.PASSTHROUGH && group.blendMode !== BlendMode.NORMAL)) return false;
        try {
            var id=stringIDToTypeID, ref=new ActionReference();
            ref.putIdentifier(id("layer"),group.id);
            var desc=executeActionGet(ref);
            function flag(name) {var k=id(name);return desc.hasKey(k) && desc.getBoolean(k);}
            if (flag("hasUserMask") || flag("hasVectorMask") || desc.hasKey(id("layerEffects"))) return false;
        } catch (_) {return false;}
        return true;
    }
    function adjustment(layer) {
        return layer.typename==="ArtLayer" && /BRIGHTNESSCONTRAST|CHANNELMIXER|COLORBALANCE|COLORLOOKUP|CURVES|EXPOSURE|GRADIENTMAP|HUESATURATION|INVERSION|LEVELS|PHOTOFILTER|POSTERIZE|SELECTIVECOLOR|THRESHOLD|VIBRANCE|BLACKANDWHITE/i.test(String(layer.kind));
    }
    function isolate(holder,first,last) {
        for (var i=0;i<holder.layers.length;i++) {
            var layer=holder.layers[i];
            if (i>last || (i<first && !(adjustment(layer) && !layer.grouped))) layer.visible=false;
        }
    }
    function photoshop(src) {
        var old=app.displayDialogs;
        app.displayDialogs=DialogModes.NO;
        function part(indices, first, last, group) {
            var copy=src.duplicate();
            try {
                var holder=copy;
                for (var k=0;k<indices.length;k++) {
                    isolate(holder,indices[k],indices[k]);
                    holder=holder.layers[indices[k]];
                }
                isolate(holder,first,last);
                var base=holder.layers[last], name=base.name, b=base.bounds;
                var x=Math.max(0,Math.floor(pixels(b[0]))), y=Math.max(0,Math.floor(pixels(b[1])));
                var w=Math.min(pixels(copy.width),Math.ceil(pixels(b[2])))-x;
                var h=Math.min(pixels(copy.height),Math.ceil(pixels(b[3])))-y;
                if (w<=0 || h<=0) return;
                var scale=1;
                for (var j=first;j<=last;j++)
                    if (holder.layers[j].visible) scale=Math.max(scale,smartScale(copy,holder.layers[j]));
                copy.crop([UnitValue(x,"px"),UnitValue(y,"px"),UnitValue(x+w,"px"),UnitValue(y+h,"px")]);
                var path=nextFile();
                savePS(copy,path,scale);
                record(path,name,group,x,y,w,h);
            } finally { copy.close(SaveOptions.DONOTSAVECHANGES); }
        }
        function walk(holder, indices, group) {
            for (var i=holder.layers.length-1;i>=0;i--) {
                var layer=holder.layers[i], top=i;
                while (top>0 && holder.layers[top-1].typename==="ArtLayer" && holder.layers[top-1].grouped) top--;
                if (layer.visible) {
                    if (layer.typename==="LayerSet" && top===i && splitPSGroup(layer)) walk(layer,indices.concat([i]),groupPath(group,layer.name));
                    else if (layer.typename==="LayerSet" || /NORMAL|BACKGROUND|TEXT|SMARTOBJECT|SOLIDFILL|GRADIENTFILL|PATTERNFILL|VIDEO|LAYER3D/i.test(String(layer.kind)))
                        part(indices,top,i,group);
                }
                i=top;
            }
        }
        try {
            var ref=src.duplicate();
            try { savePS(ref,"composite.png",1); } finally { ref.close(SaveOptions.DONOTSAVECHANGES); }
            walk(src,[],"");
        } finally { app.displayDialogs=old; }
        return src.name;
    }

    function originalPixels(item) {
        // Linked image headers retain pixel dimensions even when Illustrator
        // presents a placed file in points at its embedded print resolution.
        try {
            if (item.typename!=="PlacedItem") return null;
            var f=item.file; f.encoding="BINARY";
            if (!f.open("r")) return null;
            var s; try {s=f.read(131072);}finally{f.close();}
            function u(i){return s.charCodeAt(i)&255;}
            function u16(i){return u(i)*256+u(i+1);}
            function u32(i){return u16(i)*65536+u16(i+2);}
            if (s.substr(1,3)==="PNG") return [u32(16),u32(20)];
            if (u16(0)===65496) {
                var i=2;
                while (i+9<s.length && u(i)===255) {
                    var marker=u(i+1), len=u16(i+2);
                    if ((marker>=192 && marker<=195) || (marker>=197 && marker<=199) || (marker>=201 && marker<=203))
                        return [u16(i+7),u16(i+5)];
                    if (len<2) break;
                    i+=len+2;
                }
            }
        } catch (_) {}
        return null;
    }
    function aiScale(item) {
        if (item.typename==="GroupItem") {
            var best=300/72;
            for (var i=0;i<item.pageItems.length;i++)
                if (item.pageItems[i].parent===item && !item.pageItems[i].hidden) best=Math.max(best,aiScale(item.pageItems[i]));
            return best;
        }
        if (item.typename!=="PlacedItem" && item.typename!=="RasterItem") return 300/72;
        try {
            var m=item.matrix, sx=Math.sqrt(m.mValueA*m.mValueA+m.mValueB*m.mValueB);
            var sy=Math.sqrt(m.mValueC*m.mValueC+m.mValueD*m.mValueD);
            var px=originalPixels(item), b=item.boundingBox;
            if (px && b) return Math.max(1, px[0]/Math.max(1,Math.abs(b[2]-b[0])*sx),px[1]/Math.max(1,Math.abs(b[1]-b[3])*sy));
            return Math.max(1,1/Math.max(.001,sx),1/Math.max(.001,sy),300/72);
        } catch (_) {return 300/72;}
    }
    function illustrator(src) {
        var ab=src.artboards[src.artboards.getActiveArtboardIndex()], board=ab.artboardRect;
        function render(doc, path, w, h, wanted) {
            var opts=new ExportOptionsPNG24();
            opts.artBoardClipping=true; opts.transparency=true; opts.antiAliasing=true;
            opts.horizontalScale=opts.verticalScale=100*scaleFor(w,h,wanted);
            // Illustrator supplies the extension.
            doc.exportFile(file(path.replace(/\.png$/,"")), ExportType.PNG24, opts);
        }
        function leaf(item,group) {
            if (item.hidden || item.guides) return;
            if (item.typename==="GroupItem" && !item.clipped && item.opacity===100 && item.blendingMode===BlendModes.NORMAL) {
                for (var j=item.pageItems.length-1;j>=0;j--)
                    if (item.pageItems[j].parent===item) leaf(item.pageItems[j],groupPath(group,item.name || "组"));
                return;
            }
            var b=item.visibleBounds, l=Math.max(board[0],b[0]), t=Math.min(board[1],b[1]);
            var r=Math.min(board[2],b[2]), bot=Math.max(board[3],b[3]), w=r-l,h=t-bot;
            if (w<=0 || h<=0) return;
            var temp=app.documents.add(src.documentColorSpace);
            try {
                var obj=item.duplicate(temp.layers[0],ElementPlacement.PLACEATBEGINNING);
                obj.locked=false;
                var copied=obj.visibleBounds, origin=temp.artboards[0].artboardRect;
                obj.translate(origin[0]-copied[0],origin[1]-copied[1]);
                var localL=origin[0]+l-b[0], localT=origin[1]+t-b[1];
                temp.artboards[0].artboardRect=[localL,localT,localL+w,localT-h];
                var path=nextFile(); render(temp,path,w,h,aiScale(item));
                record(path,item.name || item.typename,group,l-board[0],board[1]-t,w,h);
            } finally {temp.close(SaveOptions.DONOTSAVECHANGES);}
        }
        render(src,"composite.png",board[2]-board[0],board[1]-board[3],300/72);
        // Document.pageItems follows the stack across layers. Descend into
        // groups once, preserving clipped groups as single transparent units.
        for (var i=src.pageItems.length-1;i>=0;i--) {
            var item=src.pageItems[i], parent=item.parent, names=[],visible=true;
            if (parent.typename!=="Layer") continue;
            while(parent.typename==="Layer"){names.unshift(parent.name);visible=visible && parent.visible;parent=parent.parent;}
            if (visible) leaf(item,names.join(" / "));
        }
        return src.name+" · "+ab.name;
    }
    try {
        if (!app.documents.length) throw Error("没有打开的文档");
        var src=app.activeDocument, ps=/photoshop/i.test(app.name);
        var board=ps?null:src.artboards[src.artboards.getActiveArtboardIndex()].artboardRect;
        var w=ps?pixels(src.width):board[2]-board[0], h=ps?pixels(src.height):board[1]-board[3];
        var name;
        if (ps) name=photoshop(src);
        else {
            var priorCoordinates=app.coordinateSystem;
            try {
                app.coordinateSystem=CoordinateSystem.DOCUMENTCOORDINATESYSTEM;
                name=illustrator(src);
            } finally {app.coordinateSystem=priorCoordinates;}
        }
        write("manifest.json",'{"version":1,"host":'+quote(ps?"photoshop":"illustrator")+',"name":'+quote(name)+',"width":'+w+',"height":'+h+',"layers":['+rows.join(",")+']}');
    } catch (e) {write("error.txt", String(e));}
    write("finished", "1");
})();
