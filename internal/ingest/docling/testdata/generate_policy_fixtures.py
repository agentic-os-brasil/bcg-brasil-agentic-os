"""Synthetic PFMT fixtures; pinned xlwt is a test-only, unshipped input."""
import datetime
import hashlib
import io
from pathlib import Path
import struct
import sys
import zipfile

out = Path(__file__).parent / "policy"
out.mkdir(exist_ok=True)
wheel = Path(sys.argv[1])
assert wheel.name == "xlwt-1.3.0-py2.py3-none-any.whl"
assert hashlib.sha256(wheel.read_bytes()).hexdigest() == "a082260524678ba48a297d922cc385f58278b8aa68741596a87de01a9c628b2e"
sys.path.insert(0, str(wheel))
import xlwt

def put(name, body):
    (out/name).write_bytes(body.encode("utf-8") if isinstance(body, str) else body)

put("literal.txt", '# title is literal\n<tag k="v">ação</tag>\n  `code` and ``` fence\n\tend  \n')
put("generic.json", '{ "name": "ação", "items": [1, true, null, {"escaped": "a\\nb"}] }\n')
put("generic.xml", '<?xml version="1.0"?><r xmlns:p="urn:test" id="7">before<p:b a="v">ação</p:b>after</r>\n')
put("quoted.csv", 'Code,Description,Note\n001,"comma, value","line one\nline two"\n002,"say ""hello""",fim\n')
html = '<!doctype html><html><body><h1>Policy HTML</h1><p>Ação local</p><table><tr><th>Item</th><th>Count</th></tr><tr><td>beta</td><td>13</td></tr></table><img src="https://127.0.0.1:9/no-fetch.png"><img src="file:///MAESTRO_POLICY_BAIT.svg"></body></html>'
put("page.html", html); put("page.htm", html)
put("structured.md", '# Policy Markdown\n\n- first\n- second\n\n| Name | Value |\n| --- | --- |\n| gamma | 17 |\n\n```text\n<tag>literal_code</tag>\n```\n\n![bait](file:///MAESTRO_POLICY_BAIT.svg)\n')

def archive(name, members):
    with zipfile.ZipFile(out/name, "w", compression=zipfile.ZIP_STORED) as z:
        for path, text in members.items():
            info=zipfile.ZipInfo(path, date_time=(2026,1,1,0,0,0)); info.external_attr=0o100644<<16
            z.writestr(info, text)

archive("spine.epub", {
    "mimetype":"application/epub+zip",
    "META-INF/container.xml":'<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container" version="1.0"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>',
    "OEBPS/content.opf":'<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="id"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>Spine proof</dc:title><dc:identifier id="id">synthetic-pfmt</dc:identifier><dc:language>en</dc:language></metadata><manifest><item id="first" href="z-first.xhtml" media-type="application/xhtml+xml"/><item id="second" href="a-second.xhtml" media-type="application/xhtml+xml"/></manifest><spine><itemref idref="first"/><itemref idref="second"/></spine></package>',
    "OEBPS/a-second.xhtml":'<html xmlns="http://www.w3.org/1999/xhtml"><body><h1>Second chapter</h1><p>AFTER_SPINE</p></body></html>',
    "OEBPS/z-first.xhtml":'<html xmlns="http://www.w3.org/1999/xhtml"><body><h1>First chapter</h1><p>BEFORE_SPINE</p></body></html>',
})
ns='http://schemas.openxmlformats.org/spreadsheetml/2006/main'
archive("sheets.xlsx", {
    "[Content_Types].xml":'<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/worksheets/sheet2.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>',
    "_rels/.rels":'<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>',
    "xl/workbook.xml":f'<workbook xmlns="{ns}" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Metrics" sheetId="1" r:id="rId1"/><sheet name="Details" sheetId="2" r:id="rId2"/></sheets></workbook>',
    "xl/_rels/workbook.xml.rels":'<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet2.xml"/></Relationships>',
    "xl/worksheets/sheet1.xml":f'<worksheet xmlns="{ns}"><dimension ref="A1:C3"/><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>Merged heading</t></is></c><c r="C1" t="inlineStr"><is><t>Cache</t></is></c></row><row r="2"><c r="A2" t="inlineStr"><is><t>delta</t></is></c><c r="B2"><v>23</v></c><c r="C2"><f>B2+6</f><v>29</v></c></row><row r="3"><c r="A3" t="b"><v>1</v></c><c r="C3"><f>99+1</f><v></v></c></row></sheetData><mergeCells count="1"><mergeCell ref="A1:B1"/></mergeCells></worksheet>',
    "xl/worksheets/sheet2.xml":f'<worksheet xmlns="{ns}"><dimension ref="A1:B2"/><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>Code</t></is></c><c r="B1" t="inlineStr"><is><t>Value</t></is></c></row><row r="2"><c r="A2" t="inlineStr"><is><t>0007</t></is></c><c r="B2"><v>3.5</v></c></row></sheetData></worksheet>',
})
book=xlwt.Workbook(); sheet=book.add_sheet("Legacy")
for col,value in enumerate(["Code","Count","Date","Cached","Empty"]): sheet.write(0,col,value)
sheet.write(1,0,"0009"); sheet.write(1,1,31)
sheet.write(1,2,datetime.datetime(2024,2,29),xlwt.easyxf(num_format_str='YYYY-MM-DD'))
sheet.write(1,3,xlwt.Formula("B2+6"))
other=book.add_sheet("Notes"); other.write(0,0,"Label"); other.write(0,1,"Value"); other.write(1,0,"legacy-note"); other.write(1,1,4.25)
buffer=io.BytesIO(); book.save(buffer); body=bytearray(buffer.getvalue())
# Replace exactly one synthetic FORMULA cache with the declared literal 37.
# This creates a cache fixture; no formula engine or Office process is invoked.
matches=[]
for offset in range(len(body)-26):
    record,size=struct.unpack_from('<HH',body,offset)
    if record==6 and 22<=size<=100 and struct.unpack_from('<HH',body,offset+4)==(1,3): matches.append(offset)
assert len(matches)==1, matches
body[matches[0]+10:matches[0]+18]=struct.pack('<d',37.0)
put("legacy.xls",body)
empty=xlwt.Workbook();empty.add_sheet("No data");buffer=io.BytesIO();empty.save(buffer);put("empty.xls",buffer.getvalue())
uncached=xlwt.Workbook();uncached.add_sheet("No cache").write(0,0,xlwt.Formula("1+1"));buffer=io.BytesIO();uncached.save(buffer);put("uncached.xls",buffer.getvalue())
with zipfile.ZipFile(out/"sheets.xlsx") as z:
    members={n:z.read(n).decode("utf-8") for n in z.namelist()}
for name in list(members):
    if name.startswith("xl/worksheets/"):
        members[name]=f'<worksheet xmlns="{ns}"><dimension ref="A1"/><sheetData/></worksheet>'
archive("empty.xlsx",members)
print('\n'.join(f'{p.name} {p.stat().st_size} {hashlib.sha256(p.read_bytes()).hexdigest()}' for p in sorted(out.iterdir())))
