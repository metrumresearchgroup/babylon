$PROBLEM 1 by MDL2PharmML v.6.0"

$INPUT  ID TIME=TIME GROUP=DROP DV MDV EVID AMT RATE AGE WT CLCR
$DATA ../Simulated_DatasetMeropenem.csv IGNORE=@
$SUBS ADVAN13 TOL=9

$MODEL
COMP (COMP1) 	;CENTRAL
COMP (COMP2) 	;PERIPHERAL



$PK
POP_CL = THETA(1)
POP_V1 = THETA(2)
POP_Q = THETA(3)
POP_V2 = THETA(4)
COV_CL_AGE = THETA(5)
COV_V1_WT = THETA(6)
RUV_PROP = THETA(7)
RUV_ADD = THETA(8)
COV_CL_CLCR = THETA(9)

ETA_CL =  ETA(1)
ETA_V1 =  ETA(2)
ETA_Q =  ETA(3)
ETA_V2 =  ETA(4)

LOGTWT = LOG((WT/70))

LOGTAGE = LOG((AGE/35))

LOGTCLCR = LOG((CLCR/83))


MU_1 = LOG(POP_CL) + COV_CL_AGE * LOGTAGE + COV_CL_CLCR * LOGTCLCR
CL =  EXP(MU_1 +  ETA(1)) ;

MU_2 = LOG(POP_V1) + COV_V1_WT * LOGTWT
V1 =  EXP(MU_2 +  ETA(2)) ;

MU_3 = LOG(POP_Q)
Q =  EXP(MU_3 +  ETA(3)) ;

MU_4 = LOG(POP_V2)
V2 =  EXP(MU_4 +  ETA(4)) ;

A_0(1) = 0
A_0(2) = 0

$DES
CENTRAL_DES = A(1)
PERIPHERAL_DES = A(2)
CC_DES = (CENTRAL_DES/V1)
DADT(1) = ((((-(Q)*CENTRAL_DES)/V1)+((Q*PERIPHERAL_DES)/V2))-((CL*CENTRAL_DES)/V1))
DADT(2) = (((Q*CENTRAL_DES)/V1)-((Q*PERIPHERAL_DES)/V2))

$ERROR
CENTRAL = A(1)
PERIPHERAL = A(2)
CC = (CENTRAL/V1)
IPRED = CC
W = SQRT((RUV_ADD*RUV_ADD)+ (RUV_PROP*RUV_PROP*IPRED*IPRED))
Y = IPRED+W*EPS(1)
IRES = DV - IPRED
IWRES = IRES/W

$THETA
( 0.0 , 14.6 )	;POP_CL
( 0.0 , 10.8 )	;POP_V1
( 0.0 , 18.6 )	;POP_Q
( 0.0 , 12.6 )	;POP_V2
(-0.34 )	;COV_CL_AGE
(0.99 )	;COV_V1_WT
(0.19 )	;RUV_PROP
(0.47 )	;RUV_ADD
(0.62  FIX )	;COV_CL_CLCR

$OMEGA
(0.118 )	;PPV_CL
(0.143 )	;PPV_V1
(0.29 )	;PPV_Q
(0.102 )	;PPV_V2

$SIGMA
1.0 FIX


$EST METHOD=COND INTER NSIG=3 SIGL=9 MAXEVALS=9999 PRINT=10 NOABORT

$COV

$TABLE  ID TIME MDV EVID AMT RATE AGE WT CLCR PRED IPRED RES IRES WRES IWRES Y DV NOAPPEND NOPRINT FILE=sdtab

$TABLE  ID CL V1 Q V2 ETA_CL ETA_V1 ETA_Q ETA_V2 NOAPPEND NOPRINT FILE=patab

$TABLE  ID AGE WT CLCR LOGTWT LOGTAGE LOGTCLCR NOAPPEND NOPRINT FILE=cotab

