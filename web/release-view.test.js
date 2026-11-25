const {test}=require("node:test");
const assert=require("node:assert/strict");
const {scenarios,reviewState,comparisonStale,sourceChanges}=require("./release-view.js");
const draft={id:"d",revision:"head"}, production={release_id:"live",generation:2};
const review={draft_id:"d",revision:"head",base_release:"live",base_generation:2,status:"pending"};
test("review eligibility follows saved source and production pins, while local edits stay separate",()=>{
 assert.equal(reviewState(review,draft,production,false).blocked,false);
 assert.match(reviewState(review,{...draft,revision:"new"},production,false).text,/source changed/);
 assert.match(reviewState(review,draft,{...production,generation:3},false).text,/active release changed/);
 assert.match(reviewState(review,draft,production,true).text,/Browser edits are not saved/);
 assert.equal(reviewState({...review,status:"rejected"},draft,production,false).blocked,true);
});
test("scenario sets are named, bounded, normalized, and explicit",()=>{
 assert.deepEqual(scenarios('[{"name":" Parcel ","call":" Shipping.quote(1) "}]'),[{name:"Parcel",call:"Shipping.quote(1)"}]);
 for (const text of ['[]','{}','[{"name":"x","call":"f(1)"},{"name":" x ","call":"f(2)"}]','[{"name":"x","call":"f(1)","extra":true}]']) assert.throws(()=>scenarios(text));
});
test("comparison staleness ignores operation tokens and notices scenario, source, and release changes",()=>{
 const rows=[{name:"Parcel",call:"f(1)"}];
 const report={draft_id:"d",draft_revision:"head",release_id:"live",generation:2,scenarios:rows};
 assert.equal(comparisonStale(report,{...draft,state_token:"changed"},production,false,JSON.stringify(rows,null,2)),"");
 assert.match(comparisonStale(report,draft,production,false,'[]'),/invalid/);
 assert.match(comparisonStale(report,draft,production,true,JSON.stringify(rows)),/unsaved/);
 assert.match(comparisonStale(report,draft,{...production,generation:3},false,JSON.stringify(rows)),/active release/);
});
test("review source comparison preserves exact literal whitespace and deleted files",()=>{
 const before={"x.star":'x = "a  b"',"gone.star":"old","questions.txt":"queued"};
 const after={"x.star":'x = "a b"',"new.star":"new","questions.txt":"different"};
 const changes=sourceChanges(before,after);
 assert.deepEqual(changes.map(x=>x.name),["gone.star","new.star","x.star"]);
 assert.equal(changes[2].before,'x = "a  b"');assert.equal(changes[0].after,"(file deleted)");
});
