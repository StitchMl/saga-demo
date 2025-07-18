import { createContext, useContext, useState } from "react";

const FlowContext = createContext({
    flow: "choreographed",
    setFlow: () => {}
});

export const useFlow = () => useContext(FlowContext);

export const FlowProvider = ({ children }) => {
    const [flow, setFlow] = useState(
        sessionStorage.getItem("flow") || "choreographed"
    );

    const update = (f) => {
        setFlow(f);
        sessionStorage.setItem("flow", f);
    };

    return (
        <FlowContext.Provider value={{ flow, setFlow: update }}>
            {children}
        </FlowContext.Provider>
    );
};