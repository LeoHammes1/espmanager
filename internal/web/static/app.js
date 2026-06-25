const source = new EventSource("/events");

source.onmessage = async () => {
  const res = await fetch("/partials/devices");
  if (res.ok) {
    document.getElementById("devices").innerHTML = await res.text();
  }
};
