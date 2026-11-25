// Pure presentation helpers shared by the release panel and its tests.
(function(root) {
  "use strict";
  function scenarios(text) {
    const parsed=JSON.parse(text);
    if (!Array.isArray(parsed) || !parsed.length || parsed.length>32) throw new Error("Enter 1–32 named scenarios as a JSON array.");
    const names=new Set();
    return parsed.map(s=>{
      if (!s || typeof s.name!=="string" || typeof s.call!=="string" || Object.keys(s).some(k=>!["name","call"].includes(k))) throw new Error("Each scenario needs only a name and a call.");
      const name=s.name.trim(), call=s.call.trim();
      if (!name || name.length>120 || !call || call.length>4096 || names.has(name)) throw new Error("Use distinct nonempty names and nonempty calls (up to 4096 characters).");
      names.add(name);return {name,call};
    });
  }
  function reviewState(review,draft,production,dirty) {
    if (!review) return {blocked:true,text:"Select a review, or check saved source and request one."};
    if (review.status!=="pending") return {blocked:true,text:`Review ${review.status}${review.reviewer ? " by "+review.reviewer : ""}${review.reason ? ": "+review.reason : "."}`};
    const reasons=[];
    if (!draft || review.draft_id!==draft.id) reasons.push("Open this review's draft.");
    else if (review.revision!==draft.revision) reasons.push("Saved source changed; request a new review.");
    if (review.base_generation!==production.generation || review.base_release!==production.release_id) reasons.push("The active release changed; request a review against the new production base.");
    if (dirty) reasons.push("Browser edits are not saved. Save them and request a new review, or retain them while inspecting this saved snapshot.");
    return {blocked:reasons.length>0,text:reasons.join(" ") || "Pending review of the checked saved source. Choose a different actor to approve or reject."};
  }
  function comparisonStale(report,draft,production,dirty,text) {
    if (!report) return "";
    const reasons=[];
    if (!draft || report.draft_id!==draft.id || report.draft_revision!==draft.revision) reasons.push("saved draft changed");
    if (report.generation!==production.generation || report.release_id!==production.release_id) reasons.push("active release changed");
    if (dirty) reasons.push("browser edits are unsaved");
    try { if (JSON.stringify(scenarios(text))!==JSON.stringify(report.scenarios)) reasons.push("scenario set changed"); }
    catch { reasons.push("scenario set is invalid"); }
    return reasons.length ? "Comparison is from earlier inputs: "+reasons.join("; ")+". Run a new comparison when ready." : "";
  }
  function sourceChanges(before,after) {
    return [...new Set([...Object.keys(before),...Object.keys(after)])].filter(n=>n.endsWith(".star") && before[n]!==after[n]).sort().map(name=>({name,before:before[name]??"(file added)",after:after[name]??"(file deleted)"}));
  }
  const api={scenarios,reviewState,comparisonStale,sourceChanges};
  root.ParkReleaseView=api;
  if (typeof module!=="undefined" && module.exports) module.exports=api;
})(globalThis);
