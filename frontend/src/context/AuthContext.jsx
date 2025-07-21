import PropTypes from "prop-types";
import {
    createContext,
    useContext,
    useEffect,
    useState,
    useMemo
} from "react";
import { setCustomerIdHeader } from "../api";

const AuthContext = createContext({
    customerId: "",
    login: (_id) => {},
    logout: () => {}
});

export const useAuth = () => useContext(AuthContext);

export const AuthProvider = ({ children }) => {
    const [customerId, setCustomerId] = useState(
        sessionStorage.getItem("customerId") || ""
    );

    useEffect(() => {
        setCustomerIdHeader(customerId);
        if (customerId) sessionStorage.setItem("customerId", customerId);
        else sessionStorage.removeItem("customerId");
    }, [customerId]);

    const login = (id) => setCustomerId(id);
    const logout = () => setCustomerId("");

    const value = useMemo(
        () => ({ customerId, login, logout }),
        [customerId]
    );

    return (
        <AuthContext.Provider value={value}>
            {children}
        </AuthContext.Provider>
    );
};

AuthProvider.propTypes = {
    children: PropTypes.node.isRequired
};