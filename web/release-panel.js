(function(root) {
  "use strict";
  function initialize(hooks) {
    const $=id=>document.getElementById(id), view=root.ParkReleaseView;
    let busy=false, production={release_id:"",generation:0}, reviews=[], releases=[], report=null, live=null, draftID=null, downloadURL=null, shownReview=null, latestDraft=null;
    const short=id=>id ? id.slice(0,12) : "none";
    const message=text=>{ $("release-message").textContent=text; };
    const selected=()=>reviews.find(r=>r.id===$("release-review").value);
    const dirty=()=>hooks.client().dirty(hooks.files()) || hooks.pending();
    function node(tag,text,props={}) {
      const n=document.createElement(tag);if (text!==undefined) n.textContent=text;
      for (const [k,v] of Object.entries(props)) n.setAttribute(k,v);
      return n;
    }
    function seedScenarios(c) {
      const id=c.draft?.id || "";
      if (draftID===id) return;
      draftID=id;report=null;$("release-comparison-results").hidden=true;
      if (downloadURL) {URL.revokeObjectURL(downloadURL);downloadURL=null;}
      const defaults=c.draft?.files["shipping.star"] ? [{name:"US parcel",call:'Shipping.quote("US", 500)'},{name:"Germany parcel",call:'Shipping.quote("DE", 800)'}] : [{name:"Current request",call:hooks.call().trim() || 'WordAdd.eval("one plus two")'}];
      let text=JSON.stringify(defaults,null,2);
      try {text=root.localStorage.getItem("park-scenarios-"+id) || text;} catch { /* optional */ }
      $("release-scenarios").value=text;
    }
    function renderReview(c) {
      const review=selected();
      $("release-review-detail").hidden=!review;
      const state=view.reviewState(review,latestDraft?.id===c.draft?.id ? latestDraft : c.draft,production,dirty());
      $("release-review-state").textContent=state.text;
      $("release-approve").disabled=busy || !review || state.blocked || c.author.trim()===review.requester;
      $("release-reject").disabled=busy || !review || review.status!=="pending" || c.author.trim()===review.requester;
      if (!review) { shownReview=null; return; }
      const detailKey=JSON.stringify([review.id,review.status,review.reason]);
      if (shownReview===detailKey) return;
      shownReview=detailKey;
      $("release-review-pin").textContent=`Saved source ${short(review.revision)} · Base generation ${review.base_generation} · Requested by ${review.requester}`;
      $("release-checks").textContent=`${review.checks.checks} implementation checks · ${review.checks.records} table records · ${review.checks.failures} failures`;
      $("release-check-lines").replaceChildren(node("li",`Pinned runtime: ${review.runtime}`),...(review.checks.lines || []).map(line=>node("li",line.text)));
      $("release-function-changes").replaceChildren(...review.diff.function_changes.map(f=>node("li",`${f.qualified}: ${f.examples_added} examples added, ${f.examples_removed} removed; ${f.body_changed ? "body changed" : "body unchanged; examples or dependencies changed"}`)));
      const base=review.base_release ? releases.find(r=>r.id===review.base_release)?.files : reference;
      $("release-source-changes").replaceChildren(...(base ? view.sourceChanges(base,review.files).map(change=>{
        const detail=node("details");detail.append(node("summary",change.name));
        const pair=node("div",undefined,{class:"source-comparison"});
        for (const [label,text] of [["Before",change.before],["Reviewed source",change.after]]) {
          const side=node("div");side.append(node("h4",label),node("pre",text,{tabindex:"0","aria-label":`${change.name} ${label}`}));pair.append(side);
        }
        detail.append(pair);return detail;
      }) : [node("p","Base source unavailable; refresh before reviewing.")]));
    }
    let reference=null;
    function render() {
      const c=hooks.client();seedScenarios(c);
      $("release-production").textContent=production.release_id ? `Active release ${short(production.release_id)} · Generation ${production.generation}` : "No active release. Request and approve a checked baseline first.";
      $("release-draft-state").textContent=!c.draft ? "Open or create a saved draft above." : dirty() ? "Browser changes are unsaved. Save source before requesting review or comparing." : `Saved draft ${c.draft.name} · Source ${short(c.draft.revision)}`;
      $("release-request").disabled=busy || !c.draft || dirty();
      $("release-refresh").disabled=busy;
      $("release-review").disabled=busy;
      $("release-live-run").disabled=busy || !production.release_id;
      $("release-rollback").disabled=busy || !$("release-rollback-target").value;
      $("release-compare").disabled=busy || !c.draft || dirty() || !production.release_id;
      renderReview(c);
      $("release-comparison-stale").textContent=view.comparisonStale(report,latestDraft?.id===c.draft?.id ? latestDraft : c.draft,production,dirty(),$("release-scenarios").value);
      $("release-live-stale").textContent=live && live.generation!==production.generation ? "This answer belongs to an earlier release generation. Run live again for the current release." : "";
    }
    async function refresh(c=hooks.client()) {
      const savedID=c.draft?.id;
      const savedState=c.draft?.state_token;
      const [rs,ls,base,remote]=await Promise.all([c.reviews(),c.releases(),c.request("base"),savedID ? c.request(`drafts/${encodeURIComponent(savedID)}?history=true`) : null]);
      const status=await c.production();
      if (c!==hooks.client() || savedID!==c.draft?.id) return;
      latestDraft=remote?.draft || null;
      if (latestDraft) c.acceptMetadata(latestDraft,savedState);
      reviews=rs.reviews;releases=ls.releases;reference=base.files;production=status.production;
      const old=$("release-review").value;
      const choices=reviews.filter(r=>r.draft_id===c.draft?.id).sort((a,b)=>b.created_at.localeCompare(a.created_at));
      $("release-review").replaceChildren(node("option","Choose a review",{value:""}),...choices.map(r=>node("option",`${r.status} · ${short(r.revision)} · ${r.requester}`,{value:r.id})));
      $("release-review").value=choices.some(r=>r.id===old) ? old : choices[0]?.id || "";
      const oldTarget=$("release-rollback-target").value;
      const targets=releases.filter(r=>r.id!==production.release_id).sort((a,b)=>b.created_at.localeCompare(a.created_at));
      $("release-rollback-target").replaceChildren(node("option","Choose a previous release",{value:""}),...targets.map(r=>node("option",`${short(r.id)} · ${new Date(r.created_at).toLocaleString()}`,{value:r.id})));
      if (targets.some(r=>r.id===oldTarget)) $("release-rollback-target").value=oldTarget;
      $("release-history").replaceChildren(...(status.activations || []).slice(-10).reverse().map(a=>node("li",`${a.kind} · generation ${a.generation} · ${short(a.release_id)} · ${a.author}${a.reason ? " · "+a.reason : ""}`)));
      render();
    }
    async function action(work) {
      if (busy) return;
      const c=hooks.client();busy=true;render();
      try {await work(c);}
      catch(error) {
        if (c!==hooks.client()) return;
        message(error.status===409 ? `The saved source, production base, or runtime changed. ${error.message}. Refresh and request a new review or comparison; your browser edits are intact.` : error.message);
        if (error.status===409) {try {await refresh(c);} catch { /* preserve original error */ }}
      } finally {busy=false;render();}
    }
    $("release-request").addEventListener("click",()=>action(async c=>{
      const value=await c.requestReview(production.generation);
      if (c!==hooks.client()) return;
      await refresh(c);if (c!==hooks.client()) return;$("release-review").value=value.review.id;render();
      message("Saved source checked and pinned for review. Saving or requesting review does not activate it.");
    }));
    for (const decision of ["approve","reject"]) $("release-"+decision).addEventListener("click",()=>action(async c=>{
      const review=selected();if (!review) return;
      if (decision==="reject" && !$("release-reason").value.trim()) {message("Enter a reason before rejecting.");return;}
      const value=await c.decideReview(review.id,decision,production.generation,$("release-reason").value.trim());
      if (c!==hooks.client()) return;
      await refresh(c);if (c!==hooks.client()) return;message(decision==="approve" ? `Release ${short(value.release.id)} activated atomically. Draft edits stay separate.` : "Review rejected. Its source and check report remain inspectable.");
    }));
    $("release-refresh").addEventListener("click",()=>action(c=>refresh(c)));
    $("release-review").addEventListener("change",render);
    $("release-rollback-target").addEventListener("change",render);
    $("release-live-copy").addEventListener("click",()=>{$("release-live-call").value=hooks.call();});
    $("release-live-run").addEventListener("click",()=>action(async c=>{
      const value=await c.runLive($("release-live-call").value.trim());if (c!==hooks.client()) return;
      live=value.run;$("release-live-result").textContent=`${live.status}: ${live.output ? root.ParkView.readable(live.output) : live.message || "No answer"}\nRelease ${short(live.release_id)} · Generation ${live.generation} · Source ${short(live.revision)}`;
      render();
    }));
    $("release-rollback").addEventListener("click",()=>action(async c=>{
      const reason=$("release-reason").value.trim();if (!reason) {message("Enter a reason before rolling back.");return;}
      await c.rollback($("release-rollback-target").value,production.generation,reason);
      if (c!==hooks.client()) return;
      await refresh(c);if (c!==hooks.client()) return;message("Previous whole-project release restored. Saved draft source is unchanged.");
    }));
    $("release-scenarios").addEventListener("input",()=>{try {root.localStorage.setItem("park-scenarios-"+draftID,$("release-scenarios").value);} catch { /* optional */ }render();});
    $("release-compare").addEventListener("click",()=>action(async c=>{
      const value=await c.compare(production.generation,view.scenarios($("release-scenarios").value));if (c!==hooks.client()) return;
      report=value.comparison;
      $("release-comparison-pin").textContent=`${report.rows.length} named scenarios · Live ${short(report.release_id)} (generation ${report.generation}) → Draft ${short(report.draft_revision)}. These are compared cases, not proof for every possible input.`;
      $("release-comparison-rows").replaceChildren(...report.rows.map(row=>{
        const tr=node("tr");
        for (const text of [row.name,row.call,`${row.before.status}: ${row.before.output || row.before.message || "No answer"}`,`${row.after.status}: ${row.after.output || row.after.message || "No answer"}`,row.category.replaceAll("_"," ")]) tr.append(node("td",text));
        tr.dataset.category=row.category;return tr;
      }));
      $("release-comparison-results").hidden=false;
      if (downloadURL) URL.revokeObjectURL(downloadURL);
      downloadURL=URL.createObjectURL(new Blob([JSON.stringify(report,null,2)],{type:"application/json"}));
      $("release-comparison-download").href=downloadURL;
      await refresh(c);if (c!==hooks.client()) return;message("Comparison complete. Neither snapshot nor the question queues changed.");render();
    }));
    $("release-panel").hidden=!root.PARK_DRAFT_API;
    refresh().catch(error=>message(error.message));
    return {render,refresh};
  }
  root.ParkReleasePanel={initialize};
})(globalThis);
