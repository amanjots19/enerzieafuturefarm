const leadForm = document.getElementById("leadForm");
const formStatus = document.getElementById("formStatus");

if (leadForm && formStatus) {
  leadForm.addEventListener("submit", (event) => {
    event.preventDefault();

    const formData = new FormData(leadForm);
    const entry = {
      name: String(formData.get("name") || "").trim(),
      email: String(formData.get("email") || "").trim(),
      age: String(formData.get("age") || "").trim(),
      phone: String(formData.get("phone") || "").trim(),
      createdAt: new Date().toISOString(),
    };

    const hasEmptyField = Object.entries(entry)
      .filter(([key]) => key !== "createdAt")
      .some(([, value]) => !value);

    if (hasEmptyField) {
      formStatus.textContent = "Please fill in all fields before submitting.";
      formStatus.className = "form-status error";
      return;
    }

    const emailValid = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(entry.email);
    const phoneValid = /^[+\d\s()-]{7,20}$/.test(entry.phone);
    const ageNumber = Number(entry.age);

    if (!emailValid) {
      formStatus.textContent = "Please enter a valid email address.";
      formStatus.className = "form-status error";
      return;
    }

    if (!Number.isFinite(ageNumber) || ageNumber < 1 || ageNumber > 120) {
      formStatus.textContent = "Please enter a valid age between 1 and 120.";
      formStatus.className = "form-status error";
      return;
    }

    if (!phoneValid) {
      formStatus.textContent = "Please enter a valid contact number.";
      formStatus.className = "form-status error";
      return;
    }

    const previousLeads = JSON.parse(localStorage.getItem("enerzeia-leads") || "[]");
    previousLeads.push(entry);
    localStorage.setItem("enerzeia-leads", JSON.stringify(previousLeads));

    formStatus.textContent =
      "Thanks for your interest in Enerzeia Future Farm.";
    formStatus.className = "form-status success";
    leadForm.reset();
  });
}
