import { createContext, useContext, useState, useCallback, useMemo } from "react";
import PropTypes from "prop-types";
import { useAuth } from "./AuthContext";
import { validateCustomer } from "../api";

const FlowContext = createContext({
    flow: "choreographed",
    setFlow: async (_f) => {}
});

export const useFlow = () => useContext(FlowContext);

export const FlowProvider = ({ children }) => {
    const { customerId, logout } = useAuth();
    const [flow, setFlow] = useState(
        sessionStorage.getItem("flow") || "choreographed"
    );

    const update = useCallback(async (f) => {
        if (!f || f === flow) return;

        setFlow(f);
        sessionStorage.setItem("flow", f);

        if (customerId) {
            try {
                await validateCustomer(customerId, f);
            } catch (error) {
                console.warn(`Validation failed for customer ${customerId} in stream ${f}. Logout in progress.`, error);
                logout();
            }
        }
    }, [customerId, flow, logout]);

    const value = useMemo(() => ({ flow, setFlow: update }), [flow, update]);

    return (
        <FlowContext.Provider value={value}>
            {children}
        </FlowContext.Provider>
    );
};

FlowProvider.propTypes = {
    children: PropTypes.node.isRequired
};
