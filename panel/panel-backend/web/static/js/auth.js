document.addEventListener("DOMContentLoaded", () => {
  const toggle = document.querySelector("[data-password-toggle]");
  const passwordInput = document.getElementById("password");

  if (toggle && passwordInput) {
    toggle.addEventListener("click", () => {
      const isHidden = passwordInput.type === "password";
      passwordInput.type = isHidden ? "text" : "password";
    });
  }
});
