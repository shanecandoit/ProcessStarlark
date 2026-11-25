"use strict";

const slides = [...document.querySelectorAll(".slide")];
let current = 0;
const previous = document.querySelector("#previous");
const next = document.querySelector("#next");
const sectionLabel = document.querySelector("#section-label");
function showCurrent(index) {
  current = index;
  sectionLabel.textContent = slides[index].dataset.title;
  sectionLabel.href = `#${slides[index].id}`;
  document.querySelector("#section-count").textContent = `${index + 1} / ${slides.length}`;
  previous.disabled = index === 0;
  next.disabled = index === slides.length - 1;
}
function navigate(index) {
  const slide = slides[Math.max(0, Math.min(slides.length - 1, index))];
  history.replaceState(null, "", `#${slide.id}`);
  slide.scrollIntoView({ behavior: matchMedia("(prefers-reduced-motion: reduce)").matches ? "instant" : "smooth" });
}
previous.addEventListener("click", () => navigate(current - 1));
next.addEventListener("click", () => navigate(current + 1));
document.addEventListener("keydown", event => {
  if (event.altKey || event.ctrlKey || event.metaKey || event.shiftKey || event.target.closest("input, textarea, select, button, a, [contenteditable]")) return;
  const destination = { ArrowRight: current + 1, ArrowLeft: current - 1, PageDown: current + 1, PageUp: current - 1, Home: 0, End: slides.length - 1 }[event.key];
  if (destination !== undefined) { event.preventDefault(); navigate(destination); }
});
// Use the section crossing a line near the top, including on small screens
// where a section can be taller than the viewport.
const observer = new IntersectionObserver(entries => {
  for (const entry of entries) if (entry.isIntersecting) showCurrent(slides.indexOf(entry.target));
}, { rootMargin: "-22% 0px -76% 0px", threshold: 0 });
slides.forEach(slide => observer.observe(slide));
showCurrent(Math.max(0, slides.findIndex(slide => `#${slide.id}` === location.hash)));

const output = document.querySelector("#demo-output");
const originalOutput = output.innerHTML;
function quote(grams) {
  // The shipping sample uses integer cents and started 500 g steps.
  const cents = 1999 + 400 * Math.ceil(grams / 500);
  document.querySelector("#request-label").textContent = `Ship ${grams.toLocaleString("en-US")} g to Germany`;
  output.replaceChildren();
  const label = document.createElement("span"); label.className = "result-label"; label.textContent = grams === 800 ? "Answered using your saved fact" : "Same saved fact, a different request";
  const price = document.createElement("h3"); price.className = "price"; price.textContent = `$${(cents / 100).toFixed(2)}`;
  const why = document.createElement("p"); why.textContent = `$19.99 base + ${Math.ceil(grams / 500)} × $4.00 weight fee. The rule used your Germany → International entry.`;
  output.append(label, price, why);
}
document.querySelector("#save-answer").addEventListener("click", () => {
  document.querySelector("#germany-zone").textContent = "International";
  document.querySelector(".missing").classList.add("answered");
  document.querySelector("#answer-controls").hidden = true;
  document.querySelector("#reuse-controls").hidden = false;
  quote(800);
  document.querySelector("#reuse-answer").focus({ preventScroll: true });
});
document.querySelector("#reuse-answer").addEventListener("click", () => {
  quote(1300);
  document.querySelector("#reuse-answer").hidden = true;
  document.querySelector("#reset-demo").focus({ preventScroll: true });
});
document.querySelector("#reset-demo").addEventListener("click", () => {
  output.innerHTML = originalOutput;
  document.querySelector("#request-label").textContent = "Ship 800 g to Germany";
  document.querySelector("#germany-zone").textContent = "Not answered yet";
  document.querySelector(".missing").classList.remove("answered");
  document.querySelector("#answer-controls").hidden = false;
  document.querySelector("#reuse-controls").hidden = true;
  document.querySelector("#reuse-answer").hidden = false;
  document.querySelector("#save-answer").focus({ preventScroll: true });
});
document.querySelector("#print-story").addEventListener("click", () => window.print());
