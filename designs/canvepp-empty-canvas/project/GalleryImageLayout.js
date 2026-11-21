(() => {
  const stressImages = [
    { id: 1, original: 435872, name: "马赛湾 · 全景", w: 4800, h: 800, src: "/_blob/4d263165acc629723540ab0895a5b9b0", note: "6:1 裁幅" },
    { id: 2, original: 39557, name: "暮色山林 · 竖幅", w: 640, h: 3840, src: "/_blob/35832844d6029e45a1f719aca97eed43", note: "1:6 裁幅" },
    { id: 3, original: 436534, name: "玫瑰 · 32 px", w: 32, h: 32, src: "/_blob/a89b3aaa0b282427ad97218888b3e5e1", note: "缩小样本" },
    { id: 4, original: 436534, name: "玫瑰 · 640 px", w: 640, h: 640, src: "/_blob/8161bec2163e48d4c58824e38b17f910", note: "同图大尺寸" },
    { id: 5, original: 45434, name: "神奈川冲浪里 · 超宽", w: 4800, h: 400, src: "/_blob/668d1f039a91b6d7f31ef1643d8a38bd", note: "12:1 裁幅" }
  ];

  function withStressImages(images) {
    const [panorama, strip, tiny, large, extraWide] = stressImages.map(({ original, ...sample }) => ({
      ...images.find(image => image.id === original), ...sample
    }));
    const companion = images.find(image => image.id === 55433);
    const remaining = images.filter(image => image !== companion);
    return [panorama, strip, companion, tiny, large, ...remaining.slice(0, 7), extraWide, ...remaining.slice(7)];
  }

  const ratio = photo => photo.w / photo.h;
  const small = photo => Math.max(photo.w, photo.h) <= 64;
  const boxStyles = (width, height) => "width:" + width + "px;height:" + height + "px";

  function tile(photo, x, y, width, height) {
    const scale = Math.min(width / photo.w, height / photo.h, small(photo) ? 2 : Infinity);
    return {
      ...photo,
      tileStyle: boxStyles(width, height) + ";left:" + x + "px;top:" + y + "px",
      imageStyle: boxStyles(photo.w * scale, photo.h * scale),
      tiny: small(photo)
    };
  }

  function rowWidth(photo, height, hitWidth) {
    const proportional = ratio(photo) * height;
    const content = small(photo) ? Math.min(proportional, photo.w * 2 + 12) : proportional;
    return Math.max(hitWidth, content);
  }

  function widthAt(photos, height, gap, hitWidth) {
    return photos.reduce((sum, photo) => sum + rowWidth(photo, height, hitWidth), 0) + (photos.length - 1) * gap;
  }

  function fitHeight(photos, width, gap, hitWidth, maxHeight) {
    let low = 0, high = maxHeight;
    for (let step = 0; step < 22; step++) {
      const middle = (low + high) / 2;
      if (widthAt(photos, middle, gap, hitWidth) > width) high = middle;
      else low = middle;
    }
    return low;
  }

  function masonry(photos, width, compact) {
    const gap = 5;
    const columns = Math.max(1, Math.floor((width + gap) / (compact ? 139 : 156)));
    const columnWidth = (width - (columns - 1) * gap) / columns;
    const bottoms = Array(columns).fill(0);
    const holes = [];
    const tiles = [];

    for (let i = 0; i < photos.length; i++) {
      const photo = photos[i];
      const aspect = ratio(photo);
      const span = aspect >= 3.2 ? Math.min(columns, Math.max(2, Math.ceil(aspect / 2))) : 1;
      const slotWidth = span * columnWidth + (span - 1) * gap;
      const partner = photos[i + 1];
      const pair = aspect <= 1 / 3.2 && columnWidth > 100 && partner && !small(partner) &&
        ratio(partner) >= .55 && ratio(partner) <= 1.5;
      const heightLimit = small(photo) ? 76 : 250;
      const height = pair
        ? fitHeight([photo, partner], slotWidth, gap, 34, heightLimit)
        : Math.max(38, Math.min(heightLimit, slotWidth / aspect));

      let start = 0, top = 0, best = Infinity;
      for (let column = 0; column <= columns - span; column++) {
        const neighbors = bottoms.slice(column, column + span);
        const y = Math.max(...neighbors);
        const unused = neighbors.reduce((area, bottom) => area + y - bottom, 0);
        const cost = y + unused / span * .75;
        if (cost < best) {
          best = cost;
          start = column;
          top = y;
        }
      }
      const hole = span === 1 ? holes.find(slot => slot.top < top && slot.end - slot.top >= height + gap) : null;
      if (hole) {
        start = hole.column;
        top = hole.top;
        hole.top += height + gap;
      } else {
        for (let column = start; column < start + span; column++) {
          if (top - bottoms[column] >= 43) holes.push({ column, top: bottoms[column], end: top });
          bottoms[column] = top + height + gap;
        }
      }
      const x = start * (columnWidth + gap);
      if (pair) {
        const stripWidth = Math.max(34, height * aspect);
        tiles.push(tile(photo, x, top, stripWidth, height));
        tiles.push(tile(partner, x + stripWidth + gap, top, slotWidth - stripWidth - gap, height));
        i++;
      } else {
        tiles.push(tile(photo, x, top, slotWidth, height));
      }
    }
    return { tiles, height: Math.max(0, Math.max(...bottoms) - gap) };
  }

  function justified(photos, width) {
    const gap = 4;
    const hitWidth = Math.min(36, width);
    const target = Math.max(83, Math.min(154, width / 5.3));
    const cap = target * 1.22;
    const tiles = [];
    let pending = [], y = 0;

    function finishRow(row, panorama = false) {
      if (!row.length) return;
      let maxHeight = panorama ? Math.max(cap, width / ratio(row[0])) : cap;
      if (row.every(small)) {
        maxHeight = Math.min(maxHeight, Math.max(42, ...row.map(photo => photo.h * 2 + 12)));
      }
      const contentHeight = fitHeight(row, width, gap, hitWidth, maxHeight);
      const height = Math.max(38, contentHeight);
      let x = 0;
      for (const photo of row) {
        const w = panorama ? width : rowWidth(photo, contentHeight, hitWidth);
        tiles.push(tile(photo, x, y, w, height));
        x += w + gap;
      }
      y += height + gap;
    }

    const following = [...photos];
    for (let i = 0; i < following.length; i++) {
      const photo = following[i];
      if (ratio(photo) >= 5) {
        // Complete an interrupted row using nearby frames before the panorama.
        while (pending.length && widthAt(pending, cap, gap, hitWidth) < width &&
            i + 1 < following.length && ratio(following[i + 1]) < 5) {
          const [next] = following.splice(i + 1, 1);
          pending.push(next);
        }
        finishRow(pending);
        finishRow([photo], true);
        pending = [];
        continue;
      }
      if (pending.length && widthAt([...pending, photo], target, gap, hitWidth) > width) {
        const available = width - widthAt(pending, target, gap, hitWidth);
        if (available <= width * .17 || (pending.length + 1) * hitWidth + pending.length * gap > width) {
          finishRow(pending);
          pending = [];
        }
      }
      pending.push(photo);
      if (widthAt(pending, target, gap, hitWidth) >= width) {
        finishRow(pending);
        pending = [];
      }
    }
    finishRow(pending);
    return { tiles, height: Math.max(0, y - gap) };
  }

  window.CanveppGalleryImageLayout = {
    withStressImages,
    arrange(photos, width, variant) {
      return variant === "justified" ? justified(photos, width) : masonry(photos, width, variant === "split");
    }
  };
})();
