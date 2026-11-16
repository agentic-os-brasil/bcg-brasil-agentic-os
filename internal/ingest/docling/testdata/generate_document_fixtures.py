"""Small synthetic model-free oracles. Standard library only; no Office/model runtime."""
from pathlib import Path
import hashlib, struct, zipfile
out=Path(__file__).parent/'documents';out.mkdir(exist_ok=True)
def rc4(key,data):
 s=list(range(256));j=0
 for i in range(256): j=(j+s[i]+key[i%len(key)])%256;s[i],s[j]=s[j],s[i]
 i=j=0; result=bytearray()
 for b in data:
  i=(i+1)%256;j=(j+s[i])%256;s[i],s[j]=s[j],s[i];result.append(b^s[(s[i]+s[j])%256])
 return bytes(result)
def pdf(name,texts,protected=False):
 objects=[b'<< /Type /Catalog /Pages 2 0 R >>',b'']
 pageids=[];key=None;identity=hashlib.md5(b'Maestro synthetic PDF').digest()
 if protected:
  pad=bytes.fromhex('28bf4e5e4e758a4164004e56fffa01082e2e00b6d0683e802f0ca9fe6453697a')
  padded=lambda p:(p+pad)[:32]
  owner=rc4(hashlib.md5(padded(b'owner')).digest()[:5],padded(b'secret'))
  key=hashlib.md5(padded(b'secret')+owner+struct.pack('<i',-4)+identity).digest()[:5]
  user=rc4(key,pad)
 for text in texts:
  n=len(objects)+1;pageids.append(n)
  objects.append(f'<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 << /Type /Font /Subtype /Type1 /BaseFont /Helvetica >> >> >> /Contents {n+1} 0 R >>'.encode())
  stream=(f'BT /F1 18 Tf 72 700 Td ({text}) Tj ET' if text else '').encode()
  if key: stream=rc4(hashlib.md5(key+(n+1).to_bytes(3,'little')+b'\0\0').digest()[:10],stream)
  objects.append(f'<< /Length {len(stream)} >>\nstream\n'.encode()+stream+b'\nendstream')
 objects[1]=f'<< /Type /Pages /Count {len(texts)} /Kids [{" ".join(str(n)+" 0 R" for n in pageids)}] >>'.encode()
 enc=''
 if protected:
  objects.append(f'<< /Filter /Standard /V 1 /R 2 /O <{owner.hex()}> /U <{user.hex()}> /P -4 >>'.encode());enc=f' /Encrypt {len(objects)} 0 R /ID [<{identity.hex()}> <{identity.hex()}>]'
 data=bytearray(b'%PDF-1.4\n%\xe2\xe3\xcf\xd3\n');offsets=[]
 for i,obj in enumerate(objects,1): offsets.append(len(data));data+=f'{i} 0 obj\n'.encode()+obj+b'\nendobj\n'
 xref=len(data);data+=f'xref\n0 {len(objects)+1}\n0000000000 65535 f \n'.encode()
 for n in offsets:data+=f'{n:010} 00000 n \n'.encode()
 data+=f'trailer\n<< /Size {len(objects)+1} /Root 1 0 R{enc} >>\nstartxref\n{xref}\n%%EOF\n'.encode();(out/name).write_bytes(data)
pdf('native.pdf',['MAESTRO NATIVE PAGE ONE 731','MAESTRO NATIVE PAGE TWO 947'])
pdf('nontext.pdf',['']);pdf('mixed.pdf',['MAESTRO NATIVE PAGE ONE 731','']);pdf('protected.pdf',['MAESTRO PROTECTED 503'],True)
p='http://schemas.openxmlformats.org/presentationml/2006/main';a='http://schemas.openxmlformats.org/drawingml/2006/main';r='http://schemas.openxmlformats.org/officeDocument/2006/relationships';pkg='http://schemas.openxmlformats.org/package/2006/relationships';ct='http://schemas.openxmlformats.org/package/2006/content-types'
rels=lambda rows:f'<Relationships xmlns="{pkg}">'+''.join(f'<Relationship Id="{i}" Type="{r}/{t}" Target="{v}"/>' for i,t,v in rows)+'</Relationships>'
shape=lambda text:f'<p:sp><p:nvSpPr><p:cNvPr id="2" name="TextBox"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="914400" y="914400"/><a:ext cx="7000000" cy="914400"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr><p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:r><a:t>{text}</a:t></a:r></a:p></p:txBody></p:sp>'
table='<p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id="3" name="Table"/><p:cNvGraphicFramePr/><p:nvPr/></p:nvGraphicFramePr><p:xfrm><a:off x="914400" y="2500000"/><a:ext cx="6000000" cy="1600000"/></p:xfrm><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/table"><a:tbl><a:tblPr/><a:tblGrid><a:gridCol w="3000000"/><a:gridCol w="3000000"/></a:tblGrid>'
for row in [['Item','Count'],['alpha','41']]:
 table+='<a:tr h="800000">'+''.join(f'<a:tc><a:txBody><a:bodyPr/><a:lstStyle/><a:p><a:r><a:t>{cell}</a:t></a:r></a:p></a:txBody><a:tcPr/></a:tc>' for cell in row)+'</a:tr>'
table+='</a:tbl></a:graphicData></a:graphic></p:graphicFrame>'
slide=lambda text,extra='':f'<p:sld xmlns:p="{p}" xmlns:a="{a}" xmlns:r="{r}"><p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/>{shape(text)}{extra}</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sld>'
members={'[Content_Types].xml':f'<Types xmlns="{ct}"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/>'+''.join(f'<Override PartName="/ppt/slides/slide{i}.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>' for i in [1,2])+'</Types>','_rels/.rels':rels([('rId1','officeDocument','ppt/presentation.xml')]),'ppt/presentation.xml':f'<p:presentation xmlns:p="{p}" xmlns:r="{r}"><p:sldIdLst><p:sldId id="256" r:id="rId1"/><p:sldId id="257" r:id="rId2"/></p:sldIdLst><p:sldSz cx="9144000" cy="6858000"/><p:notesSz cx="6858000" cy="9144000"/></p:presentation>','ppt/_rels/presentation.xml.rels':rels([('rId1','slide','slides/slide1.xml'),('rId2','slide','slides/slide2.xml')]),'ppt/slides/slide1.xml':slide('MAESTRO POWERPOINT FIRST 421',table),'ppt/slides/slide2.xml':slide('MAESTRO POWERPOINT SECOND 683')}
with zipfile.ZipFile(out/'native.pptx','w',compression=zipfile.ZIP_STORED) as z:
 for name,text in members.items():
  info=zipfile.ZipInfo(name,date_time=(2026,1,1,0,0,0));info.external_attr=0o100644<<16;z.writestr(info,text)
for f in sorted(out.iterdir()):print(f.name,len(f.read_bytes()),hashlib.sha256(f.read_bytes()).hexdigest())
