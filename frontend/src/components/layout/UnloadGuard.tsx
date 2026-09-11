import { useEffect } from "react";
import { useSoftphoneContext } from "@/softphone/SoftphoneContext";

// UnloadGuard shows the browser's native "leave site?" confirmation when the page
// is refreshed or closed while the softphone is active, so an accidental F5 or
// tab close does not silently drop an ongoing call or the SIP registration. It is
// mounted inside the SoftphoneProvider so it can read the live call state.
export default function UnloadGuard() {
  const phone = useSoftphoneContext();
  // Guard whenever the softphone is engaged (registered, ringing, or in a call).
  // When there is no softphone (status "disabled") there is nothing to lose.
  const engaged = phone.status !== "disabled" && phone.status !== "error";

  useEffect(() => {
    if (!engaged) return;
    const onBeforeUnload = (e: BeforeUnloadEvent) => {
      e.preventDefault();
      e.returnValue = ""; // required by Chrome/Firefox to trigger the prompt
      return "";
    };
    window.addEventListener("beforeunload", onBeforeUnload);
    return () => window.removeEventListener("beforeunload", onBeforeUnload);
  }, [engaged]);

  return null;
}
